//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/poyrazk/cloudtalk/internal/testutil/itest"
)

func TestWSPresenceOnlineOfflineBroadcast(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	app := itest.BuildRealtimeLoopbackApp(env.Pool)
	t.Cleanup(app.Close)

	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	u1 := registerAndLogin(t, ts.URL, "presence-alice")
	u2 := registerAndLogin(t, ts.URL, "presence-bob")

	c1 := wsDial(t, ts.URL, u1.AccessToken)
	defer c1.Close()

	c2 := wsDial(t, ts.URL, u2.AccessToken)

	if !waitPresenceForUser(t, c1, u2.UserID.String(), "online", 5*time.Second) {
		t.Fatal("did not receive online presence event for second user")
	}

	_ = c2.Close()
	if !waitPresenceForUser(t, c1, u2.UserID.String(), "offline", 5*time.Second) {
		t.Fatal("did not receive offline presence event for second user")
	}
}

func waitPresenceForUser(t *testing.T, conn *websocket.Conn, userID, status string, timeout time.Duration) bool {
	t.Helper()
	return waitForWSEvent(conn, timeout, func(env wsEnvelope) bool {
		if env.Type != "presence" {
			return false
		}
		var payload map[string]any
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false
		}
		return payload["user_id"] == userID && payload["status"] == status
	})
}
