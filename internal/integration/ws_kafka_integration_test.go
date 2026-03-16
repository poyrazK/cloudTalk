//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/poyrazk/cloudtalk/internal/handler"
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

func TestWSRoomTypingExcludesSender(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	app := itest.BuildRealtimeLoopbackApp(env.Pool)
	t.Cleanup(app.Close)

	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	u1 := registerAndLogin(t, ts.URL, "typing-room-alice")
	u2 := registerAndLogin(t, ts.URL, "typing-room-bob")
	u3 := registerAndLogin(t, ts.URL, "typing-room-charlie")

	createRoomResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms", u1.AccessToken, map[string]string{
		"name":        "typing-room",
		"description": "integration",
	})
	if createRoomResp.StatusCode != http.StatusCreated {
		t.Fatalf("create room status: got=%d", createRoomResp.StatusCode)
	}
	room := decodeJSON[model.Room](t, createRoomResp)

	for _, u := range []authUser{u2, u3} {
		joinResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms/"+room.ID.String()+"/join", u.AccessToken, nil)
		if joinResp.StatusCode != http.StatusNoContent {
			t.Fatalf("join room status for %s: got=%d", u.UserID.String(), joinResp.StatusCode)
		}
	}

	c1 := wsDial(t, ts.URL, u1.AccessToken)
	defer c1.Close()
	c2 := wsDial(t, ts.URL, u2.AccessToken)
	defer c2.Close()
	c3 := wsDial(t, ts.URL, u3.AccessToken)
	defer c3.Close()

	for _, c := range []*websocket.Conn{c1, c2, c3} {
		if err := c.WriteJSON(map[string]string{"type": "join_room", "room_id": room.ID.String()}); err != nil {
			t.Fatalf("join_room send: %v", err)
		}
	}

	if err := c1.WriteJSON(map[string]any{"type": "typing", "room_id": room.ID.String(), "typing": true}); err != nil {
		t.Fatalf("typing true send: %v", err)
	}

	gotReceiver := waitForWSEvent(c2, 5*time.Second, func(env wsEnvelope) bool {
		if env.Type != "typing" {
			return false
		}
		var payload map[string]any
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false
		}
		return payload["user_id"] == u1.UserID.String() && payload["room_id"] == room.ID.String() && payload["typing"] == true
	})
	if !gotReceiver {
		t.Fatal("receiver did not get room typing start")
	}

	if got := waitForWSEvent(c1, 500*time.Millisecond, func(env wsEnvelope) bool { return env.Type == "typing" }); got {
		t.Fatal("sender unexpectedly received room typing event")
	}

	if err := c1.WriteJSON(map[string]any{"type": "typing", "room_id": room.ID.String(), "typing": false}); err != nil {
		t.Fatalf("typing false send: %v", err)
	}
	gotStop := waitForWSEvent(c3, 5*time.Second, func(env wsEnvelope) bool {
		if env.Type != "typing" {
			return false
		}
		var payload map[string]any
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false
		}
		return payload["user_id"] == u1.UserID.String() && payload["room_id"] == room.ID.String() && payload["typing"] == false
	})
	if !gotStop {
		t.Fatal("room typing stop event not received")
	}
}

func TestWSRoomTypingRequiresMembership(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	app := itest.BuildRealtimeLoopbackApp(env.Pool)
	t.Cleanup(app.Close)

	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	owner := registerAndLogin(t, ts.URL, "typing-room-owner")
	member := registerAndLogin(t, ts.URL, "typing-room-member")
	outsider := registerAndLogin(t, ts.URL, "typing-room-outsider")

	createRoomResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms", owner.AccessToken, map[string]string{
		"name":        "typing-room-membership",
		"description": "integration",
	})
	if createRoomResp.StatusCode != http.StatusCreated {
		t.Fatalf("create room status: got=%d", createRoomResp.StatusCode)
	}
	room := decodeJSON[model.Room](t, createRoomResp)

	joinResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms/"+room.ID.String()+"/join", member.AccessToken, nil)
	if joinResp.StatusCode != http.StatusNoContent {
		t.Fatalf("join room status: got=%d", joinResp.StatusCode)
	}

	cMember := wsDial(t, ts.URL, member.AccessToken)
	defer cMember.Close()
	cOutsider := wsDial(t, ts.URL, outsider.AccessToken)
	defer cOutsider.Close()

	for _, c := range []*websocket.Conn{cMember, cOutsider} {
		if err := c.WriteJSON(map[string]string{"type": "join_room", "room_id": room.ID.String()}); err != nil {
			t.Fatalf("join_room send: %v", err)
		}
	}

	if err := cOutsider.WriteJSON(map[string]any{"type": "typing", "room_id": room.ID.String(), "typing": true}); err != nil {
		t.Fatalf("outsider typing send: %v", err)
	}

	if got := waitForWSEvent(cMember, 700*time.Millisecond, func(env wsEnvelope) bool { return env.Type == "typing" }); got {
		t.Fatal("member unexpectedly received typing from outsider")
	}
}

func TestWSDMTypingRecipientOnly(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	app := itest.BuildRealtimeLoopbackApp(env.Pool)
	t.Cleanup(app.Close)

	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	sender := registerAndLogin(t, ts.URL, "typing-dm-sender")
	receiver := registerAndLogin(t, ts.URL, "typing-dm-receiver")
	third := registerAndLogin(t, ts.URL, "typing-dm-third")

	cSender := wsDial(t, ts.URL, sender.AccessToken)
	defer cSender.Close()
	cReceiver := wsDial(t, ts.URL, receiver.AccessToken)
	defer cReceiver.Close()
	cThird := wsDial(t, ts.URL, third.AccessToken)
	defer cThird.Close()

	if err := cSender.WriteJSON(map[string]any{"type": "typing_dm", "to": receiver.UserID.String(), "typing": true}); err != nil {
		t.Fatalf("sender typing_dm true: %v", err)
	}

	gotTypingStart := waitForWSEvent(cReceiver, 5*time.Second, func(env wsEnvelope) bool {
		if env.Type != "typing_dm" {
			return false
		}
		var payload map[string]any
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false
		}
		return payload["user_id"] == sender.UserID.String() && payload["to_user_id"] == receiver.UserID.String() && payload["typing"] == true
	})
	if !gotTypingStart {
		t.Fatal("receiver did not get typing_dm start event")
	}

	if got := waitForWSEvent(cSender, 400*time.Millisecond, func(env wsEnvelope) bool { return env.Type == "typing_dm" }); got {
		t.Fatal("sender unexpectedly received typing_dm event")
	}
	if got := waitForWSEvent(cThird, 400*time.Millisecond, func(env wsEnvelope) bool { return env.Type == "typing_dm" }); got {
		t.Fatal("third user unexpectedly received typing_dm event")
	}

	if err := cSender.WriteJSON(map[string]any{"type": "typing_dm", "to": receiver.UserID.String(), "typing": false}); err != nil {
		t.Fatalf("sender typing_dm false: %v", err)
	}
	gotTypingStop := waitForWSEvent(cReceiver, 5*time.Second, func(env wsEnvelope) bool {
		if env.Type != "typing_dm" {
			return false
		}
		var payload map[string]any
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false
		}
		return payload["user_id"] == sender.UserID.String() && payload["to_user_id"] == receiver.UserID.String() && payload["typing"] == false
	})
	if !gotTypingStop {
		t.Fatal("receiver did not get typing_dm stop event")
	}
}

func TestWSDMTypingSelfIgnored(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	app := itest.BuildRealtimeLoopbackApp(env.Pool)
	t.Cleanup(app.Close)

	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	user := registerAndLogin(t, ts.URL, "typing-dm-self")
	conn := wsDial(t, ts.URL, user.AccessToken)
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{"type": "typing_dm", "to": user.UserID.String(), "typing": true}); err != nil {
		t.Fatalf("self typing_dm write: %v", err)
	}
	if got := waitForWSEvent(conn, 500*time.Millisecond, func(env wsEnvelope) bool { return env.Type == "typing_dm" }); got {
		t.Fatal("self typing_dm should not produce events")
	}
}

func TestWSRoomSubscriptionThrottleRejectsBurstWithErrorEvent(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	app := itest.BuildRealtimeLoopbackAppWithThrottle(env.Pool, handler.NewWSThrottleConfig(10, 10, 10, 10, 10, 10, 1, 1))
	t.Cleanup(app.Close)

	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	user := registerAndLogin(t, ts.URL, "throttle-room-join-user")

	conn := wsDial(t, ts.URL, user.AccessToken)
	defer conn.Close()

	throttled := false
	roomID := model.Room{ID: uuid.New()}.ID.String()
	for i := 0; i < 12; i++ {
		if err := conn.WriteJSON(map[string]any{"type": "join_room", "room_id": roomID}); err != nil {
			t.Fatalf("write burst join_room: %v", err)
		}
	}
	throttled = waitForWSEvent(conn, 2*time.Second, func(env wsEnvelope) bool {
		if env.Type != "error" {
			return false
		}
		var payload map[string]any
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false
		}
		return payload["code"] == "rate_limited"
	})
	if !throttled {
		t.Fatal("expected join_room burst to be rate limited")
	}
}

func TestWSTypingThrottleDropsBurstSilently(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	app := itest.BuildRealtimeLoopbackAppWithThrottle(env.Pool, handler.NewWSThrottleConfig(10, 10, 1, 1, 10, 10, 10, 10))
	t.Cleanup(app.Close)

	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	u1 := registerAndLogin(t, ts.URL, "throttle-typing-alice")
	u2 := registerAndLogin(t, ts.URL, "throttle-typing-bob")

	createRoomResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms", u1.AccessToken, map[string]string{
		"name":        "typing-throttle-room",
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

	for _, c := range []*websocket.Conn{c1, c2} {
		if err := c.WriteJSON(map[string]string{"type": "join_room", "room_id": room.ID.String()}); err != nil {
			t.Fatalf("join_room send: %v", err)
		}
	}

	typingEvents := 0
	for i := 0; i < 8; i++ {
		if err := c1.WriteJSON(map[string]any{"type": "typing", "room_id": room.ID.String(), "typing": true}); err != nil {
			t.Fatalf("typing write: %v", err)
		}
		waitForWSEvent(c2, 40*time.Millisecond, func(env wsEnvelope) bool {
			if env.Type != "typing" {
				return false
			}
			typingEvents++
			return true
		})
	}

	if typingEvents >= 8 {
		t.Fatalf("expected some typing events to be dropped, got %d", typingEvents)
	}
	if typingEvents == 0 {
		t.Fatal("expected typing fanout before throttling, got zero delivered typing events")
	}
	if got := waitForWSEvent(c1, 150*time.Millisecond, func(env wsEnvelope) bool { return env.Type == "error" }); got {
		t.Fatal("typing throttle should not emit error event")
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
