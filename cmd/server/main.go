package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	authsvc "github.com/poyrazk/cloudtalk/internal/auth"
	"github.com/poyrazk/cloudtalk/internal/config"
	"github.com/poyrazk/cloudtalk/internal/db"
	"github.com/poyrazk/cloudtalk/internal/handler"
	"github.com/poyrazk/cloudtalk/internal/hub"
	"github.com/poyrazk/cloudtalk/internal/kafka"
	"github.com/poyrazk/cloudtalk/internal/logger"
	"github.com/poyrazk/cloudtalk/internal/metrics"
	"github.com/poyrazk/cloudtalk/internal/repository"
	"github.com/poyrazk/cloudtalk/internal/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	logger.Init()
	if err := run(); err != nil {
		slog.Error("startup", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validate: %w", err)
	}

	// --- Database ---
	ctx := context.Background()
	pool, err := db.Connect(ctx, db.ConnectConfig{
		DSN:         cfg.DatabaseDSN,
		MaxConns:    cfg.DBMaxConns,
		MinConns:    cfg.DBMinConns,
		MaxConnLife: time.Duration(cfg.DBMaxConnLife) * time.Second,
		MaxConnIdle: time.Duration(cfg.DBMaxConnIdle) * time.Second,
	})
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}
	defer pool.Close()

	if err := db.Migrate(cfg.DatabaseDSN, "file://migrations"); err != nil {
		return fmt.Errorf("db migrate: %w", err)
	}

	// --- Repositories ---
	userRepo := repository.NewUserRepo(pool)
	roomRepo := repository.NewRoomRepo(pool)
	msgRepo := repository.NewMessageRepo(pool)

	// --- Auth ---
	auth := authsvc.NewService(userRepo, cfg.JWTSecret, cfg.JWTExpMinutes, cfg.RefreshExpDays)

	// --- Kafka producer ---
	producer, err := kafka.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		return fmt.Errorf("kafka producer: %w", err)
	}
	defer producer.Close()
	if err := producer.VerifyTopics(ctx, kafka.RequiredTopics()); err != nil {
		return fmt.Errorf("kafka topic verification: %w", err)
	}

	// --- Hub ---
	h := hub.New()

	// --- Services ---
	presenceSvc := service.NewPresenceService(producer, h, userRepo)
	roomSvc := service.NewRoomServiceWithPresence(roomRepo, presenceSvc)
	msgSvc := service.NewMessageServiceWithPresence(roomRepo, msgRepo, userRepo, producer, presenceSvc)

	// --- Kafka consumer fan-out ---
	topics := []string{kafka.TopicRoomMessages, kafka.TopicDMMessages, kafka.TopicPresence}
	consumer, err := kafka.NewConsumer(cfg.KafkaBrokers, cfg.KafkaGroupID, topics, func(topic string, evt kafka.ChatEvent) {
		fanOut(h, topic, evt)
	})
	if err != nil {
		return fmt.Errorf("kafka consumer: %w", err)
	}
	consumerCtx, cancelConsumer := context.WithCancel(ctx)
	consumer.Start(consumerCtx)

	// Background job: purge expired refresh tokens daily.
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := userRepo.DeleteExpiredRefreshTokens(consumerCtx); err != nil {
					slog.Error("token cleanup", "err", err)
				}
			case <-consumerCtx.Done():
				return
			}
		}
	}()
	defer func() {
		cancelConsumer()
		consumer.Close()
	}()

	startDBPoolMetrics(consumerCtx, pool)

	// --- Handlers ---
	authH := handler.NewAuthHandler(auth)
	roomH := handler.NewRoomHandler(roomSvc, msgSvc)
	dmH := handler.NewDMHandler(msgSvc)
	wsH := handler.NewWSHandler(auth, h, roomSvc, msgSvc, presenceSvc, producer, cfg.AllowedOrigins)

	// --- Router ---
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(handler.Observability())

	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(handler.RateLimit(cfg.RateLimit))
			r.Post("/auth/register", authH.Register)
			r.Post("/auth/login", authH.Login)
		})
		r.Post("/auth/refresh", authH.Refresh)
		r.Post("/auth/logout", authH.Logout)

		r.Group(func(r chi.Router) {
			r.Use(authsvc.Middleware(auth))

			r.Post("/rooms", roomH.Create)
			r.Get("/rooms", roomH.List)
			r.Get("/rooms/conversations", roomH.Conversations)
			r.Get("/rooms/unread-counts", roomH.UnreadCounts)
			r.Get("/rooms/{id}", roomH.Get)
			r.Post("/rooms/{id}/join", roomH.Join)
			r.Post("/rooms/{id}/leave", roomH.Leave)
			r.Get("/rooms/{id}/members", roomH.Members)
			r.Get("/rooms/{id}/messages", roomH.Messages)

			r.Get("/dms/{userId}/messages", dmH.Messages)
			r.Get("/dms/unread-counts", dmH.UnreadCounts)
			r.Get("/dms/conversations", dmH.Conversations)
		})
	})

	r.Get("/ws", wsH.ServeHTTP)
	registerObservabilityRoutes(r, pool, producer, consumer)

	// --- Server ---
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("cloudTalk listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "err", err)
		}
	}()

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "err", err)
	}

	return nil
}

func startDBPoolMetrics(ctx context.Context, pool *pgxpool.Pool) {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			metrics.UpdateDBPoolStats(pool.Stat())
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
}

func registerObservabilityRoutes(r chi.Router, pool *pgxpool.Pool, producer *kafka.Producer, consumer *kafka.Consumer) {
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			slog.Error("health write", "err", err)
		}
	})
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		checkCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(checkCtx); err != nil {
			http.Error(w, "database not ready", http.StatusServiceUnavailable)
			return
		}
		if err := producer.Ping(checkCtx); err != nil {
			http.Error(w, "kafka not ready", http.StatusServiceUnavailable)
			return
		}
		if err := consumer.ReadyError(); err != nil {
			http.Error(w, "consumer not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ready")); err != nil {
			slog.Error("ready write", "err", err)
		}
	})
}

// fanOut routes a Kafka ChatEvent to the correct Hub broadcast method.
func fanOut(h *hub.Hub, topic string, evt kafka.ChatEvent) {
	switch topic {
	case kafka.TopicRoomMessages:
		roomID, err := uuid.Parse(evt.RoomID)
		if err != nil {
			return
		}
		senderID, senderErr := uuid.Parse(evt.SenderID)
		out, _ := json.Marshal(map[string]interface{}{
			"type":    evt.Type,
			"payload": evt.Payload,
		})
		if evt.Type == "typing" && senderErr == nil {
			h.BroadcastRoomExcept(roomID, senderID, hub.Event{Data: out})
			return
		}
		h.BroadcastRoom(roomID, hub.Event{Data: out})

	case kafka.TopicDMMessages:
		toID, err := uuid.Parse(evt.ToUserID)
		if err != nil {
			return
		}
		out, _ := json.Marshal(map[string]interface{}{
			"type":    evt.Type,
			"payload": evt.Payload,
		})
		h.BroadcastUser(toID, hub.Event{Data: out})
		if evt.Type == "typing_dm" {
			return
		}
		// also deliver to sender's local client if connected here
		senderID, err := uuid.Parse(evt.SenderID)
		if err == nil && senderID != toID {
			h.BroadcastUser(senderID, hub.Event{Data: out})
		}

	case kafka.TopicPresence:
		out, _ := json.Marshal(map[string]interface{}{
			"type":    "presence",
			"payload": evt.Payload,
		})
		h.BroadcastAll(hub.Event{Data: out})
	}
}
