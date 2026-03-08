package hub

import (
	"testing"

	"github.com/google/uuid"
)

func TestHubRegisterBroadcastAndUnregister(t *testing.T) {
	h := New()
	userID := uuid.New()
	roomID := uuid.New()
	c := NewClient(userID)
	c.JoinRoom(roomID)
	h.Register(c)

	if !h.IsOnline(userID) {
		t.Fatal("expected user online")
	}

	evt := Event{Data: []byte(`{"type":"message"}`)}
	h.BroadcastRoom(roomID, evt)
	select {
	case got := <-c.Send:
		if string(got.Data) != string(evt.Data) {
			t.Fatalf("unexpected room event: %s", string(got.Data))
		}
	default:
		t.Fatal("expected room event delivery")
	}

	h.BroadcastUser(userID, evt)
	select {
	case <-c.Send:
	default:
		t.Fatal("expected user event delivery")
	}

	h.Unregister(c)
	if h.IsOnline(userID) {
		t.Fatal("expected user offline")
	}
}
