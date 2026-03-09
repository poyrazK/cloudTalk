//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/poyrazk/cloudtalk/internal/model"
	"github.com/poyrazk/cloudtalk/internal/testutil/itest"
)

func TestWSRoomMessageFanout(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	app := itest.BuildRealtimeLoopbackApp(env.Pool)
	t.Cleanup(app.Close)

	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	u1 := registerAndLogin(t, ts.URL, "ws-alice")
	u2 := registerAndLogin(t, ts.URL, "ws-bob")

	createRoomResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms", u1.AccessToken, map[string]string{
		"name":        "live-room",
		"description": "integration",
	})
	if createRoomResp.StatusCode != http.StatusCreated {
		t.Fatalf("create room status: got=%d", createRoomResp.StatusCode)
	}
	room := decodeJSON[model.Room](t, createRoomResp)

	joinResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms/"+room.ID.String()+"/join", u2.AccessToken, nil)
	if joinResp.StatusCode != http.StatusNoContent {
		t.Fatalf("join room status: got=%d", joinResp.StatusCode)
	}

	c1 := wsDial(t, ts.URL, u1.AccessToken)
	defer c1.Close()
	c2 := wsDial(t, ts.URL, u2.AccessToken)
	defer c2.Close()

	if err := c1.WriteJSON(map[string]string{"type": "join_room", "room_id": room.ID.String()}); err != nil {
		t.Fatalf("c1 join_room: %v", err)
	}
	if err := c2.WriteJSON(map[string]string{"type": "join_room", "room_id": room.ID.String()}); err != nil {
		t.Fatalf("c2 join_room: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	const wantContent = "hello via ws"
	if err := c1.WriteJSON(map[string]string{"type": "message", "room_id": room.ID.String(), "content": wantContent}); err != nil {
		t.Fatalf("c1 send room message: %v", err)
	}

	got := waitForWSEvent(c2, 5*time.Second, func(env wsEnvelope) bool {
		if env.Type != "message" {
			return false
		}
		var payload map[string]any
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false
		}
		return payload["content"] == wantContent
	})
	if !got {
		t.Fatal("did not receive room message fan-out")
	}
}

func TestWSDMFanout(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	app := itest.BuildRealtimeLoopbackApp(env.Pool)
	t.Cleanup(app.Close)

	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	u1 := registerAndLogin(t, ts.URL, "dm-alice")
	u2 := registerAndLogin(t, ts.URL, "dm-bob")

	c1 := wsDial(t, ts.URL, u1.AccessToken)
	defer c1.Close()
	c2 := wsDial(t, ts.URL, u2.AccessToken)
	defer c2.Close()

	const wantContent = "private hello"
	if err := c1.WriteJSON(map[string]string{"type": "dm", "to": u2.UserID.String(), "content": wantContent}); err != nil {
		t.Fatalf("c1 send dm: %v", err)
	}

	gotReceiver := waitForWSEvent(c2, 5*time.Second, func(env wsEnvelope) bool {
		if env.Type != "dm" {
			return false
		}
		var payload map[string]any
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false
		}
		return payload["content"] == wantContent
	})

	gotSender := waitForWSEvent(c1, 5*time.Second, func(env wsEnvelope) bool {
		if env.Type != "dm" {
			return false
		}
		var payload map[string]any
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false
		}
		return payload["content"] == wantContent
	})

	if !gotReceiver || !gotSender {
		t.Fatalf("did not receive expected dm fan-out (receiver=%v sender=%v)", gotReceiver, gotSender)
	}
}

func TestWSDMReadReceiptFlow(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	app := itest.BuildRealtimeLoopbackApp(env.Pool)
	t.Cleanup(app.Close)

	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	sender := registerAndLogin(t, ts.URL, "receipt-sender")
	receiver := registerAndLogin(t, ts.URL, "receipt-receiver")

	cSender := wsDial(t, ts.URL, sender.AccessToken)
	defer cSender.Close()
	cReceiver := wsDial(t, ts.URL, receiver.AccessToken)
	defer cReceiver.Close()

	const content = "receipt flow"
	if err := cSender.WriteJSON(map[string]string{"type": "dm", "to": receiver.UserID.String(), "content": content}); err != nil {
		t.Fatalf("sender send dm: %v", err)
	}

	var dmID string
	gotReceiverDM := waitForWSEvent(cReceiver, 5*time.Second, func(env wsEnvelope) bool {
		if env.Type != "dm" {
			return false
		}
		var payload map[string]any
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false
		}
		if payload["content"] != content {
			return false
		}
		if id, ok := payload["id"].(string); ok {
			dmID = id
		}
		return dmID != ""
	})
	if !gotReceiverDM {
		t.Fatal("receiver did not get dm")
	}

	gotDeliveredReceipt := waitForWSEvent(cSender, 5*time.Second, func(env wsEnvelope) bool {
		if env.Type != "dm_receipt" {
			return false
		}
		var payload map[string]any
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false
		}
		return payload["dm_id"] == dmID && payload["delivered_at"] != nil
	})
	if !gotDeliveredReceipt {
		t.Fatal("sender did not get delivered receipt")
	}

	if err := cReceiver.WriteJSON(map[string]string{"type": "read_dm", "dm_id": dmID}); err != nil {
		t.Fatalf("receiver send read_dm: %v", err)
	}

	gotReadReceipt := waitForWSEvent(cSender, 5*time.Second, func(env wsEnvelope) bool {
		if env.Type != "dm_receipt" {
			return false
		}
		var payload map[string]any
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false
		}
		return payload["dm_id"] == dmID && payload["read_at"] != nil
	})
	if !gotReadReceipt {
		t.Fatal("sender did not get read receipt")
	}
}

type wsEnvelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func wsDial(t *testing.T, baseURL, accessToken string) *websocket.Conn {
	t.Helper()
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/ws?token=" + accessToken
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	return conn
}

func tryReadWSEvent(conn *websocket.Conn, timeout time.Duration) (wsEnvelope, error) {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return wsEnvelope{}, err
	}
	var env wsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return wsEnvelope{}, err
	}
	return env, nil
}

func waitForWSEvent(conn *websocket.Conn, timeout time.Duration, match func(wsEnvelope) bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}
		env, err := tryReadWSEvent(conn, remaining)
		if err != nil {
			return false
		}
		if match(env) {
			return true
		}
	}
}
