//go:build integration

package itest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/IBM/sarama"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	authsvc "github.com/poyrazk/cloudtalk/internal/auth"
	"github.com/poyrazk/cloudtalk/internal/handler"
	"github.com/poyrazk/cloudtalk/internal/hub"
	"github.com/poyrazk/cloudtalk/internal/kafka"
	"github.com/poyrazk/cloudtalk/internal/repository"
	"github.com/poyrazk/cloudtalk/internal/service"
)

type RealtimeApp struct {
	Router   http.Handler
	Auth     *authsvc.Service
	Rooms    *repository.RoomRepo
	Messages *repository.MessageRepo

	producer *kafka.Producer
	consumer *kafka.Consumer
	cancel   context.CancelFunc
}

type loopbackPublisher struct {
	h *hub.Hub
}

func (p loopbackPublisher) Publish(topic, _ string, evt kafka.ChatEvent) error {
	realtimeFanOut(p.h, topic, evt)
	return nil
}

func BuildRealtimeLoopbackApp(pool *pgxpool.Pool) *RealtimeApp {
	userRepo := repository.NewUserRepo(pool)
	roomRepo := repository.NewRoomRepo(pool)
	msgRepo := repository.NewMessageRepo(pool)

	auth := authsvc.NewService(userRepo, "integration-secret", 15, 7)
	h := hub.New()
	publisher := loopbackPublisher{h: h}

	roomSvc := service.NewRoomService(roomRepo)
	msgSvc := service.NewMessageService(roomRepo, msgRepo, publisher)
	presenceSvc := service.NewPresenceService(publisher, h)
	wsH := handler.NewWSHandler(auth, h, msgSvc, presenceSvc, nil, nil)
	authH := handler.NewAuthHandler(auth)
	roomH := handler.NewRoomHandler(roomSvc, msgSvc)
	dmH := handler.NewDMHandler(msgSvc)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", authH.Register)
		r.Post("/auth/login", authH.Login)
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
			r.Get("/dms/unread-counts", dmH.UnreadCounts)
		})
	})

	r.Get("/ws", wsH.ServeHTTP)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return &RealtimeApp{
		Router:   r,
		Auth:     auth,
		Rooms:    roomRepo,
		Messages: msgRepo,
	}
}

func BuildRealtimeApp(env *Env) (*RealtimeApp, error) {
	if len(env.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers are required")
	}

	userRepo := repository.NewUserRepo(env.Pool)
	roomRepo := repository.NewRoomRepo(env.Pool)
	msgRepo := repository.NewMessageRepo(env.Pool)

	if err := ensureKafkaTopics(env.Brokers); err != nil {
		return nil, fmt.Errorf("ensure kafka topics: %w", err)
	}

	auth := authsvc.NewService(userRepo, "integration-secret", 15, 7)
	h := hub.New()

	producer, err := kafka.NewProducer(env.Brokers)
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}

	roomSvc := service.NewRoomService(roomRepo)
	msgSvc := service.NewMessageService(roomRepo, msgRepo, producer)
	presenceSvc := service.NewPresenceService(producer, h)
	wsH := handler.NewWSHandler(auth, h, msgSvc, presenceSvc, producer, nil)
	authH := handler.NewAuthHandler(auth)
	roomH := handler.NewRoomHandler(roomSvc, msgSvc)
	dmH := handler.NewDMHandler(msgSvc)

	groupID := "itest-" + uuid.NewString()
	topics := []string{kafka.TopicRoomMessages, kafka.TopicDMMessages, kafka.TopicPresence}
	consumer, err := kafka.NewConsumer(env.Brokers, groupID, topics, func(topic string, evt kafka.ChatEvent) {
		realtimeFanOut(h, topic, evt)
	})
	if err != nil {
		_ = producer.Close()
		return nil, fmt.Errorf("create kafka consumer: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	consumer.Start(ctx)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", authH.Register)
		r.Post("/auth/login", authH.Login)
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
			r.Get("/dms/unread-counts", dmH.UnreadCounts)
		})
	})

	r.Get("/ws", wsH.ServeHTTP)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	app := &RealtimeApp{
		Router:   r,
		Auth:     auth,
		Rooms:    roomRepo,
		Messages: msgRepo,
		producer: producer,
		consumer: consumer,
		cancel:   cancel,
	}

	return app, nil
}

func ensureKafkaTopics(brokers []string) error {
	cfg := sarama.NewConfig()
	admin, err := sarama.NewClusterAdmin(brokers, cfg)
	if err != nil {
		return err
	}
	defer admin.Close()

	topics := []string{kafka.TopicRoomMessages, kafka.TopicDMMessages, kafka.TopicPresence}
	for _, topic := range topics {
		err := admin.CreateTopic(topic, &sarama.TopicDetail{NumPartitions: 1, ReplicationFactor: 1}, false)
		if err != nil && err != sarama.ErrTopicAlreadyExists {
			return err
		}
	}
	return nil
}

func (a *RealtimeApp) Close() {
	if a.cancel != nil {
		a.cancel()
	}
	if a.consumer != nil {
		_ = a.consumer.Close()
	}
	if a.producer != nil {
		_ = a.producer.Close()
	}
}

func realtimeFanOut(h *hub.Hub, topic string, evt kafka.ChatEvent) {
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
			"type":    evt.Type,
			"payload": evt.Payload,
		})
		h.BroadcastUser(toID, hub.Event{Data: out})

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

func WaitFor(timeout time.Duration, check func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
