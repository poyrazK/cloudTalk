package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/hub"
	"github.com/poyrazk/cloudtalk/internal/kafka"
)

type presencePayload struct {
	UserID string `json:"user_id"`
	Status string `json:"status"` // "online" | "offline"
}

// PresenceService tracks online users via Kafka.
type PresenceService struct {
	online   map[uuid.UUID]struct{}
	mu       sync.RWMutex
	producer eventPublisher
	hub      presenceHub
	users    presenceUserRepository
}

type presenceHub interface {
	BroadcastUser(userID uuid.UUID, evt hub.Event)
}

type presenceUserRepository interface {
	UpdateLastSeen(ctx context.Context, userID uuid.UUID, at time.Time) error
}

func NewPresenceService(p eventPublisher, h presenceHub, users presenceUserRepository) *PresenceService {
	return &PresenceService{
		online:   make(map[uuid.UUID]struct{}),
		producer: p,
		hub:      h,
		users:    users,
	}
}

func (s *PresenceService) SetOnline(_ context.Context, userID uuid.UUID) {
	s.mu.Lock()
	s.online[userID] = struct{}{}
	s.mu.Unlock()
	s.publishPresence(userID, "online")
}

func (s *PresenceService) SetOffline(_ context.Context, userID uuid.UUID) {
	s.mu.Lock()
	_, wasOnline := s.online[userID]
	if wasOnline {
		delete(s.online, userID)
	}
	s.mu.Unlock()
	if !wasOnline {
		return
	}

	if s.users != nil {
		writeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.users.UpdateLastSeen(writeCtx, userID, time.Now().UTC()); err != nil {
			slog.Error("update last seen", "err", err, "user_id", userID)
		}
	}
	s.publishPresence(userID, "offline")
}

func (s *PresenceService) IsOnline(userID uuid.UUID) bool {
	s.mu.RLock()
	_, ok := s.online[userID]
	s.mu.RUnlock()
	return ok
}

// HandleKafkaPresence is called by the Kafka consumer for presence events
// and fans them out to locally-connected WebSocket clients.
func (s *PresenceService) HandleKafkaPresence(evt kafka.ChatEvent) {
	s.hub.BroadcastUser(uuid.MustParse(evt.ToUserID), hub.Event{Data: mustMarshal(map[string]interface{}{
		"type":    "presence",
		"payload": evt.Payload,
	})})
}

func (s *PresenceService) publishPresence(userID uuid.UUID, status string) {
	payload, err := json.Marshal(presencePayload{UserID: userID.String(), Status: status})
	if err != nil {
		slog.Error("marshal presence", "err", err)
		return
	}
	if err := s.producer.Publish(kafka.TopicPresence, userID.String(), kafka.ChatEvent{
		Type:    "presence",
		Payload: payload,
	}); err != nil {
		slog.Error("publish presence to kafka", "err", err)
	}
}

func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
