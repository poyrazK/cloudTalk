//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/model"
	"github.com/poyrazk/cloudtalk/internal/testutil/itest"
)

func TestAuthLifecycleIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	if err := env.ResetDB(context.Background()); err != nil {
		t.Fatalf("reset db: %v", err)
	}
	app := itest.BuildHTTPApp(env.Pool)
	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	email := fmt.Sprintf("alice-%s@example.com", uuid.NewString())
	registerBody := map[string]string{
		"username": "alice",
		"email":    email,
		"password": "password123",
	}
	registerResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/auth/register", "", registerBody)
	if registerResp.StatusCode != http.StatusCreated {
		t.Fatalf("register status: got=%d", registerResp.StatusCode)
	}

	loginResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/auth/login", "", map[string]string{
		"email":    email,
		"password": "password123",
	})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status: got=%d", loginResp.StatusCode)
	}
	tokens := decodeJSON[map[string]string](t, loginResp)
	refresh := tokens["refresh_token"]
	if refresh == "" {
		t.Fatal("expected refresh token")
	}

	refreshResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/auth/refresh", "", map[string]string{"refresh_token": refresh})
	if refreshResp.StatusCode != http.StatusOK {
		t.Fatalf("refresh status: got=%d", refreshResp.StatusCode)
	}
	newAccess := decodeJSON[map[string]string](t, refreshResp)["access_token"]
	if newAccess == "" {
		t.Fatal("expected new access token")
	}

	logoutResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/auth/logout", "", map[string]string{"refresh_token": refresh})
	if logoutResp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status: got=%d", logoutResp.StatusCode)
	}

	refreshAfterLogout := doJSON(t, http.MethodPost, ts.URL+"/api/v1/auth/refresh", "", map[string]string{"refresh_token": refresh})
	if refreshAfterLogout.StatusCode != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: got=%d", refreshAfterLogout.StatusCode)
	}
}

func TestRoomAndDMHistoryAuthorizationIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	if err := env.ResetDB(context.Background()); err != nil {
		t.Fatalf("reset db: %v", err)
	}
	app := itest.BuildHTTPApp(env.Pool)
	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	u1 := registerAndLogin(t, ts.URL, "alice")
	u2 := registerAndLogin(t, ts.URL, "bob")

	createRoomResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms", u1.AccessToken, map[string]string{
		"name":        "general",
		"description": "all chat",
	})
	if createRoomResp.StatusCode != http.StatusCreated {
		t.Fatalf("create room status: got=%d", createRoomResp.StatusCode)
	}
	room := decodeJSON[model.Room](t, createRoomResp)

	forbiddenResp := doJSON(t, http.MethodGet, ts.URL+"/api/v1/rooms/"+room.ID.String()+"/messages", u2.AccessToken, nil)
	if forbiddenResp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-member room history should be forbidden: got=%d", forbiddenResp.StatusCode)
	}

	joinResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms/"+room.ID.String()+"/join", u2.AccessToken, nil)
	if joinResp.StatusCode != http.StatusNoContent {
		t.Fatalf("join room status: got=%d", joinResp.StatusCode)
	}

	msg := &model.Message{ID: uuid.New(), RoomID: room.ID, SenderID: u1.UserID, Content: "hello room"}
	if err := app.Rooms.SaveMessage(context.Background(), msg); err != nil {
		t.Fatalf("save room message: %v", err)
	}

	historyResp := doJSON(t, http.MethodGet, ts.URL+"/api/v1/rooms/"+room.ID.String()+"/messages?limit=10", u2.AccessToken, nil)
	if historyResp.StatusCode != http.StatusOK {
		t.Fatalf("room history status: got=%d", historyResp.StatusCode)
	}
	roomMsgs := decodeJSON[[]model.Message](t, historyResp)
	if len(roomMsgs) == 0 {
		t.Fatal("expected at least one room message")
	}

	dm := &model.DirectMessage{ID: uuid.New(), SenderID: u1.UserID, ReceiverID: u2.UserID, Content: "private"}
	if err := app.Messages.SaveDM(context.Background(), dm); err != nil {
		t.Fatalf("save dm: %v", err)
	}

	dmHistoryResp := doJSON(t, http.MethodGet, ts.URL+"/api/v1/dms/"+u1.UserID.String()+"/messages?limit=10", u2.AccessToken, nil)
	if dmHistoryResp.StatusCode != http.StatusOK {
		t.Fatalf("dm history status: got=%d", dmHistoryResp.StatusCode)
	}
	dms := decodeJSON[[]model.DirectMessage](t, dmHistoryResp)
	if len(dms) == 0 {
		t.Fatal("expected at least one dm")
	}
}

type authUser struct {
	UserID      uuid.UUID
	AccessToken string
}

func registerAndLogin(t *testing.T, baseURL, name string) authUser {
	t.Helper()

	email := fmt.Sprintf("%s-%s@example.com", name, uuid.NewString())
	registerResp := doJSON(t, http.MethodPost, baseURL+"/api/v1/auth/register", "", map[string]string{
		"username": name,
		"email":    email,
		"password": "password123",
	})
	if registerResp.StatusCode != http.StatusCreated {
		t.Fatalf("register %s: got=%d", name, registerResp.StatusCode)
	}
	var regUser struct {
		ID string `json:"id"`
	}
	decodeInto(t, registerResp, &regUser)
	uid, err := uuid.Parse(regUser.ID)
	if err != nil {
		t.Fatalf("parse user id: %v", err)
	}

	loginResp := doJSON(t, http.MethodPost, baseURL+"/api/v1/auth/login", "", map[string]string{
		"email":    email,
		"password": "password123",
	})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login %s: got=%d", name, loginResp.StatusCode)
	}
	tokens := decodeJSON[map[string]string](t, loginResp)

	return authUser{UserID: uid, AccessToken: tokens["access_token"]}
}

func doJSON(t *testing.T, method, url, accessToken string, body any) *http.Response {
	t.Helper()

	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		payload = b
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var out T
	decodeInto(t, resp, &out)
	return out
}

func decodeInto(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}
