package handler

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/hub"
)

func TestWSHandlerAllowClientMessageRejectsChatAfterBurst(t *testing.T) {
	t.Parallel()

	throttle := NewWSThrottleConfig(1, 1, 1, 1, 1, 1, 1, 1)
	h := &WSHandler{throttle: throttle}
	client := hub.NewClient(uuid.New(), throttle.client)

	allowed, action := h.allowClientMessage(client, "dm")
	if !allowed || action != "" {
		t.Fatalf("expected first chat event allowed, got allowed=%v action=%q", allowed, action)
	}
	allowed, action = h.allowClientMessage(client, "dm")
	if allowed || action != "reject" {
		t.Fatalf("expected second chat event rejected, got allowed=%v action=%q", allowed, action)
	}
}

func TestWSHandlerAllowClientMessageDropsTypingAfterBurst(t *testing.T) {
	t.Parallel()

	throttle := NewWSThrottleConfig(1, 1, 1, 1, 1, 1, 1, 1)
	h := &WSHandler{throttle: throttle}
	client := hub.NewClient(uuid.New(), throttle.client)

	allowed, action := h.allowClientMessage(client, "typing")
	if !allowed || action != "" {
		t.Fatalf("expected first typing event allowed, got allowed=%v action=%q", allowed, action)
	}
	allowed, action = h.allowClientMessage(client, "typing")
	if allowed || action != "drop" {
		t.Fatalf("expected second typing event dropped, got allowed=%v action=%q", allowed, action)
	}
}

func TestWSHandlerHandleThrottledEventRejectsWithErrorPayload(t *testing.T) {
	t.Parallel()

	h := &WSHandler{}
	client := hub.NewClient(uuid.New(), hub.ThrottleConfig{})

	h.handleThrottledEvent(t.Context(), client, "dm", "reject")

	select {
	case evt := <-client.Send:
		var out outgoingMsg
		if err := json.Unmarshal(evt.Data, &out); err != nil {
			t.Fatalf("unmarshal outgoing event: %v", err)
		}
		if out.Type != "error" {
			t.Fatalf("expected error event, got %q", out.Type)
		}
		var payload map[string]string
		if err := json.Unmarshal(out.Payload, &payload); err != nil {
			t.Fatalf("unmarshal error payload: %v", err)
		}
		if payload["code"] != "rate_limited" {
			t.Fatalf("expected rate_limited code, got %q", payload["code"])
		}
		if payload["message"] != "too many websocket events" {
			t.Fatalf("unexpected throttle message: %q", payload["message"])
		}
	default:
		t.Fatal("expected throttled reject event to be sent")
	}
}

func TestWSHandlerHandleThrottledEventDropsWithoutErrorPayload(t *testing.T) {
	t.Parallel()

	h := &WSHandler{}
	client := hub.NewClient(uuid.New(), hub.ThrottleConfig{})

	h.handleThrottledEvent(t.Context(), client, "typing", "drop")

	select {
	case evt := <-client.Send:
		t.Fatalf("expected no event for dropped typing throttle, got %+v", evt)
	default:
	}
}
