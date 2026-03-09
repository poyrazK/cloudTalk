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
	authsvc "github.com/poyrazk/cloudtalk/internal/auth"
	"github.com/poyrazk/cloudtalk/internal/config"
	"github.com/poyrazk/cloudtalk/internal/db"
	"github.com/poyrazk/cloudtalk/internal/handler"
	"github.com/poyrazk/cloudtalk/internal/hub"
	"github.com/poyrazk/cloudtalk/internal/kafka"
	"github.com/poyrazk/cloudtalk/internal/logger"
	"github.com/poyrazk/cloudtalk/internal/repository"
	"github.com/poyrazk/cloudtalk/internal/service"
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

	// --- Hub ---
	h := hub.New()

	// --- Services ---
	roomSvc := service.NewRoomService(roomRepo)
	msgSvc := service.NewMessageService(roomRepo, msgRepo, producer)
	presenceSvc := service.NewPresenceService(producer, h)

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

	// --- Handlers ---
	authH := handler.NewAuthHandler(auth)
	roomH := handler.NewRoomHandler(roomSvc, msgSvc)
	dmH := handler.NewDMHandler(msgSvc)
	wsH := handler.NewWSHandler(auth, h, msgSvc, presenceSvc, producer, cfg.AllowedOrigins)

	// --- Router ---
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

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
			r.Get("/rooms/{id}", roomH.Get)
			r.Post("/rooms/{id}/join", roomH.Join)
			r.Post("/rooms/{id}/leave", roomH.Leave)
			r.Get("/rooms/{id}/messages", roomH.Messages)

			r.Get("/dms/{userId}/messages", dmH.Messages)
		})
	})

	r.Get("/ws", wsH.ServeHTTP)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			slog.Error("health write", "err", err)
		}
	})

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

// fanOut routes a Kafka ChatEvent to the correct Hub broadcast method.
func fanOut(h *hub.Hub, topic string, evt kafka.ChatEvent) {
	switch topic {
	case kafka.TopicRoomMessages:
		roomID, err := uuid.Parse(evt.RoomID)
		if err != nil {
			return
		}
		out, _ := json.Marshal(map[string]interface{}{
			"type":    evt.Type,
			"payload": evt.Payload,
		})
		h.BroadcastRoom(roomID, hub.Event{Data: out})

	case kafka.TopicDMMessages:
		toID, err := uuid.Parse(evt.ToUserID)
		if err != nil {
			return
		}
		out, _ := json.Marshal(map[string]interface{}{
			"type":    "dm",
			"payload": evt.Payload,
		})
		h.BroadcastUser(toID, hub.Event{Data: out})
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
