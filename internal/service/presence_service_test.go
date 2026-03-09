package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/hub"
	"github.com/poyrazk/cloudtalk/internal/kafka"
)

type fakePresenceHub struct {
	userID uuid.UUID
	event  hub.Event
}

func (f *fakePresenceHub) BroadcastUser(userID uuid.UUID, evt hub.Event) {
	f.userID = userID
	f.event = evt
}

func TestPresenceServiceOnlineOfflineAndPublish(t *testing.T) {
	t.Parallel()

	pub := &fakePublisher{}
	h := &fakePresenceHub{}
	svc := NewPresenceService(pub, h)
	uid := uuid.New()

	svc.SetOnline(context.Background(), uid)
	if !svc.IsOnline(uid) {
		t.Fatal("expected user online")
	}
	if pub.publishN == 0 || pub.topic != kafka.TopicPresence {
		t.Fatal("expected presence publish on online")
	}

	svc.SetOffline(context.Background(), uid)
	if svc.IsOnline(uid) {
		t.Fatal("expected user offline")
	}
}

func TestPresenceServiceHandleKafkaPresence(t *testing.T) {
	t.Parallel()

	pub := &fakePublisher{}
	h := &fakePresenceHub{}
	svc := NewPresenceService(pub, h)
	uid := uuid.New()
	payload := json.RawMessage(`{"user_id":"` + uid.String() + `","status":"online"}`)

	svc.HandleKafkaPresence(kafka.ChatEvent{ToUserID: uid.String(), Payload: payload})
	if h.userID != uid {
		t.Fatalf("expected broadcast user %s, got %s", uid, h.userID)
	}
	if len(h.event.Data) == 0 {
		t.Fatal("expected broadcast payload")
	}
}
