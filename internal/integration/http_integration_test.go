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
	"time"

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

func TestDMHistoryIncludesReceiptFieldsIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	if err := env.ResetDB(context.Background()); err != nil {
		t.Fatalf("reset db: %v", err)
	}
	app := itest.BuildHTTPApp(env.Pool)
	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	sender := registerAndLogin(t, ts.URL, "receipts-sender")
	receiver := registerAndLogin(t, ts.URL, "receipts-receiver")

	dm := &model.DirectMessage{ID: uuid.New(), SenderID: sender.UserID, ReceiverID: receiver.UserID, Content: "with receipts"}
	if err := app.Messages.SaveDM(context.Background(), dm); err != nil {
		t.Fatalf("save dm: %v", err)
	}

	deliveredAt := time.Now().UTC().Add(-30 * time.Second).Truncate(time.Second)
	readAt := deliveredAt.Add(15 * time.Second)
	if err := app.Messages.MarkDMDelivered(context.Background(), dm.ID, receiver.UserID, deliveredAt); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	if err := app.Messages.MarkDMRead(context.Background(), dm.ID, receiver.UserID, readAt); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	historyResp := doJSON(t, http.MethodGet, ts.URL+"/api/v1/dms/"+receiver.UserID.String()+"/messages?limit=10", sender.AccessToken, nil)
	if historyResp.StatusCode != http.StatusOK {
		t.Fatalf("dm history status: got=%d", historyResp.StatusCode)
	}
	dms := decodeJSON[[]model.DirectMessage](t, historyResp)
	if len(dms) == 0 {
		t.Fatal("expected at least one dm")
	}
	got := dms[0]
	if got.DeliveredAt == nil {
		t.Fatal("expected delivered_at in dm history")
	}
	if got.ReadAt == nil {
		t.Fatal("expected read_at in dm history")
	}
}

func TestDMUnreadCountsIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	if err := env.ResetDB(context.Background()); err != nil {
		t.Fatalf("reset db: %v", err)
	}
	app := itest.BuildHTTPApp(env.Pool)
	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	owner := registerAndLogin(t, ts.URL, "counts-owner")
	senderA := registerAndLogin(t, ts.URL, "counts-sender-a")
	senderB := registerAndLogin(t, ts.URL, "counts-sender-b")

	now := time.Now().UTC()
	dmA1 := &model.DirectMessage{ID: uuid.New(), SenderID: senderA.UserID, ReceiverID: owner.UserID, Content: "a1", CreatedAt: now.Add(-4 * time.Minute)}
	dmA2 := &model.DirectMessage{ID: uuid.New(), SenderID: senderA.UserID, ReceiverID: owner.UserID, Content: "a2", CreatedAt: now.Add(-3 * time.Minute)}
	dmB1 := &model.DirectMessage{ID: uuid.New(), SenderID: senderB.UserID, ReceiverID: owner.UserID, Content: "b1", CreatedAt: now.Add(-2 * time.Minute)}
	outbound := &model.DirectMessage{ID: uuid.New(), SenderID: owner.UserID, ReceiverID: senderA.UserID, Content: "out", CreatedAt: now.Add(-1 * time.Minute)}

	for _, dm := range []*model.DirectMessage{dmA1, dmA2, dmB1, outbound} {
		if err := app.Messages.SaveDM(context.Background(), dm); err != nil {
			t.Fatalf("save dm %s: %v", dm.ID, err)
		}
	}

	if err := app.Messages.MarkDMRead(context.Background(), dmA1.ID, owner.UserID, now); err != nil {
		t.Fatalf("mark dm read: %v", err)
	}

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/v1/dms/unread-counts", owner.AccessToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unread counts status: got=%d", resp.StatusCode)
	}
	counts := decodeJSON[[]model.DMUnreadCount](t, resp)
	if len(counts) != 2 {
		t.Fatalf("expected 2 unread-count entries, got %d", len(counts))
	}

	got := map[uuid.UUID]int{}
	for _, c := range counts {
		got[c.UserID] = c.Count
	}
	if got[senderA.UserID] != 1 {
		t.Fatalf("expected senderA unread=1, got %d", got[senderA.UserID])
	}
	if got[senderB.UserID] != 1 {
		t.Fatalf("expected senderB unread=1, got %d", got[senderB.UserID])
	}
}

func TestDMConversationsIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	if err := env.ResetDB(context.Background()); err != nil {
		t.Fatalf("reset db: %v", err)
	}
	app := itest.BuildHTTPApp(env.Pool)
	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	owner := registerAndLogin(t, ts.URL, "conv-owner")
	p1 := registerAndLogin(t, ts.URL, "conv-p1")
	p2 := registerAndLogin(t, ts.URL, "conv-p2")

	now := time.Now().UTC()
	dms := []*model.DirectMessage{
		{ID: uuid.New(), SenderID: p1.UserID, ReceiverID: owner.UserID, Content: "old", CreatedAt: now.Add(-5 * time.Minute)},
		{ID: uuid.New(), SenderID: owner.UserID, ReceiverID: p1.UserID, Content: "latest-p1", CreatedAt: now.Add(-1 * time.Minute)},
		{ID: uuid.New(), SenderID: p2.UserID, ReceiverID: owner.UserID, Content: "latest-p2", CreatedAt: now.Add(-2 * time.Minute)},
	}
	for _, dm := range dms {
		if err := app.Messages.SaveDM(context.Background(), dm); err != nil {
			t.Fatalf("save dm %s: %v", dm.ID, err)
		}
	}

	app.Presence.SetOnline(context.Background(), p1.UserID)
	app.Presence.SetOnline(context.Background(), p2.UserID)
	app.Presence.SetOffline(context.Background(), p2.UserID)

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/v1/dms/conversations?limit=50", owner.AccessToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("conversations status: got=%d", resp.StatusCode)
	}
	var convs []model.DMConversation
	decodeInto(t, resp, &convs)
	if len(convs) != 2 {
		t.Fatalf("expected 2 conversations, got %d", len(convs))
	}
	got := map[uuid.UUID]model.DMConversation{}
	for _, c := range convs {
		got[c.UserID] = c
	}
	convP1, ok := got[p1.UserID]
	if !ok || convP1.LastMessage == nil || convP1.LastMessage.Content != "latest-p1" || convP1.Username != "conv-p1" || !convP1.Online || convP1.LastSeen != nil {
		t.Fatalf("unexpected p1 conversation: %+v", convP1)
	}
	convP2, ok := got[p2.UserID]
	if !ok || convP2.LastMessage == nil || convP2.LastMessage.Content != "latest-p2" || convP2.Username != "conv-p2" || convP2.Online || convP2.LastSeen == nil {
		t.Fatalf("unexpected p2 conversation: %+v", convP2)
	}
}

func TestRoomUnreadCountsIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	if err := env.ResetDB(context.Background()); err != nil {
		t.Fatalf("reset db: %v", err)
	}
	app := itest.BuildHTTPApp(env.Pool)
	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	owner := registerAndLogin(t, ts.URL, "room-unread-owner")
	member := registerAndLogin(t, ts.URL, "room-unread-member")

	createRoomResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms", owner.AccessToken, map[string]string{
		"name":        "room-unread",
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

	msg1 := &model.Message{ID: uuid.New(), RoomID: room.ID, SenderID: owner.UserID, Content: "one"}
	msg2 := &model.Message{ID: uuid.New(), RoomID: room.ID, SenderID: owner.UserID, Content: "two"}
	if err := app.Rooms.SaveMessage(context.Background(), msg1); err != nil {
		t.Fatalf("save message1: %v", err)
	}
	if err := app.Rooms.SaveMessage(context.Background(), msg2); err != nil {
		t.Fatalf("save message2: %v", err)
	}

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/v1/rooms/unread-counts", member.AccessToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("room unread counts status: got=%d", resp.StatusCode)
	}
	counts := decodeJSON[[]model.RoomUnreadCount](t, resp)
	if len(counts) == 0 {
		t.Fatal("expected at least one room unread count")
	}
	var found bool
	for _, c := range counts {
		if c.RoomID == room.ID {
			found = true
			if c.Count != 2 {
				t.Fatalf("expected room unread count=2, got %d", c.Count)
			}
		}
	}
	if !found {
		t.Fatal("expected unread count entry for created room")
	}
}

func TestRoomConversationsIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	ctx := context.Background()
	if err := env.ResetDB(ctx); err != nil {
		t.Fatalf("reset db: %v", err)
	}
	app := itest.BuildHTTPApp(env.Pool)
	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	owner := registerAndLogin(t, ts.URL, "room-conv-owner")
	member := registerAndLogin(t, ts.URL, "room-conv-member")

	createRoom := func(name string) model.Room {
		t.Helper()
		resp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms", owner.AccessToken, map[string]string{
			"name":        name,
			"description": "integration",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create room %s status: got=%d", name, resp.StatusCode)
		}
		return decodeJSON[model.Room](t, resp)
	}

	roomA := createRoom("room-conv-a")
	roomB := createRoom("room-conv-b")
	roomC := createRoom("room-conv-c")
	app.Presence.SetOnline(ctx, owner.UserID)
	app.Presence.SetOnline(ctx, member.UserID)
	app.Presence.SetOffline(ctx, member.UserID)

	for _, room := range []model.Room{roomA, roomB, roomC} {
		joinResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms/"+room.ID.String()+"/join", member.AccessToken, nil)
		if joinResp.StatusCode != http.StatusNoContent {
			t.Fatalf("join room %s status: got=%d", room.Name, joinResp.StatusCode)
		}
	}

	now := time.Now().UTC()
	_, err := env.Pool.Exec(ctx,
		`INSERT INTO messages (id, room_id, sender_id, content, created_at) VALUES ($1,$2,$3,$4,$5)`,
		uuid.New(), roomA.ID, owner.UserID, "a-latest", now.Add(2*time.Minute),
	)
	if err != nil {
		t.Fatalf("insert roomA message: %v", err)
	}
	_, err = env.Pool.Exec(ctx,
		`INSERT INTO messages (id, room_id, sender_id, content, created_at, deleted_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		uuid.New(), roomB.ID, owner.UserID, "b-latest-deleted", now.Add(1*time.Minute), now,
	)
	if err != nil {
		t.Fatalf("insert roomB message: %v", err)
	}

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/v1/rooms/conversations?limit=50", member.AccessToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("room conversations status: got=%d", resp.StatusCode)
	}
	conversations := decodeJSON[[]model.RoomConversation](t, resp)
	if len(conversations) != 3 {
		t.Fatalf("expected 3 room conversations, got %d", len(conversations))
	}

	if conversations[0].RoomID != roomA.ID || conversations[0].MemberCount != 2 || conversations[0].UnreadCount != 1 || conversations[0].OnlineCount != 1 || conversations[0].LastMessage == nil || conversations[0].LastMessage.Content != "a-latest" {
		t.Fatalf("unexpected first room conversation: %+v", conversations[0])
	}
	if conversations[1].RoomID != roomB.ID || conversations[1].MemberCount != 2 || conversations[1].UnreadCount != 1 || conversations[1].OnlineCount != 1 || conversations[1].LastMessage == nil || conversations[1].LastMessage.Content != "b-latest-deleted" || conversations[1].LastMessage.DeletedAt == nil {
		t.Fatalf("unexpected second room conversation: %+v", conversations[1])
	}
	if conversations[2].RoomID != roomC.ID || conversations[2].MemberCount != 2 || conversations[2].UnreadCount != 0 || conversations[2].OnlineCount != 1 || conversations[2].LastMessage != nil {
		t.Fatalf("unexpected third room conversation: %+v", conversations[2])
	}
}

func TestRoomMembersIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	ctx := context.Background()
	if err := env.ResetDB(ctx); err != nil {
		t.Fatalf("reset db: %v", err)
	}
	app := itest.BuildHTTPApp(env.Pool)
	ts := httptest.NewServer(app.Router)
	t.Cleanup(ts.Close)

	owner := registerAndLogin(t, ts.URL, "room-members-owner")
	member := registerAndLogin(t, ts.URL, "room-members-member")
	outsider := registerAndLogin(t, ts.URL, "room-members-outsider")

	createResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms", owner.AccessToken, map[string]string{
		"name":        "room-members",
		"description": "integration",
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create room status: got=%d", createResp.StatusCode)
	}
	room := decodeJSON[model.Room](t, createResp)

	joinResp := doJSON(t, http.MethodPost, ts.URL+"/api/v1/rooms/"+room.ID.String()+"/join", member.AccessToken, nil)
	if joinResp.StatusCode != http.StatusNoContent {
		t.Fatalf("join room status: got=%d", joinResp.StatusCode)
	}

	app.Presence.SetOnline(ctx, owner.UserID)

	membersResp := doJSON(t, http.MethodGet, ts.URL+"/api/v1/rooms/"+room.ID.String()+"/members", member.AccessToken, nil)
	if membersResp.StatusCode != http.StatusOK {
		t.Fatalf("room members status: got=%d", membersResp.StatusCode)
	}
	members := decodeJSON[[]model.RoomMemberDetail](t, membersResp)
	if len(members) != 2 {
		t.Fatalf("expected 2 room members, got %d", len(members))
	}

	byID := map[uuid.UUID]model.RoomMemberDetail{}
	for _, m := range members {
		byID[m.UserID] = m
	}
	ownerMember, ok := byID[owner.UserID]
	if !ok || ownerMember.Username != "room-members-owner" || !ownerMember.Online || ownerMember.JoinedAt.IsZero() || ownerMember.LastSeen != nil {
		t.Fatalf("unexpected owner member row: %+v", ownerMember)
	}
	joinedMember, ok := byID[member.UserID]
	if !ok || joinedMember.Username != "room-members-member" || joinedMember.Online || joinedMember.JoinedAt.IsZero() || joinedMember.LastSeen == nil {
		t.Fatalf("unexpected joined member row: %+v", joinedMember)
	}

	forbiddenResp := doJSON(t, http.MethodGet, ts.URL+"/api/v1/rooms/"+room.ID.String()+"/members", outsider.AccessToken, nil)
	if forbiddenResp.StatusCode != http.StatusForbidden {
		t.Fatalf("room members outsider status: got=%d", forbiddenResp.StatusCode)
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
