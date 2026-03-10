//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/model"
	"github.com/poyrazk/cloudtalk/internal/repository"
	"github.com/poyrazk/cloudtalk/internal/testutil/itest"
)

func TestRepositoryMessagePaginationIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	ctx := context.Background()
	if err := env.ResetDB(ctx); err != nil {
		t.Fatalf("reset db: %v", err)
	}

	userRepo := repository.NewUserRepo(env.Pool)
	roomRepo := repository.NewRoomRepo(env.Pool)

	user := &model.User{ID: uuid.New(), Username: "alice", Email: fmt.Sprintf("alice-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	room := &model.Room{ID: uuid.New(), Name: "general", Description: "desc", CreatedBy: user.ID}
	if err := roomRepo.Create(ctx, room); err != nil {
		t.Fatalf("create room: %v", err)
	}
	if err := roomRepo.AddMember(ctx, room.ID, user.ID); err != nil {
		t.Fatalf("add member: %v", err)
	}

	base := time.Now().UTC().Add(-1 * time.Hour)
	insertMessageAt(t, env, room.ID, user.ID, "m1", base.Add(1*time.Minute))
	insertMessageAt(t, env, room.ID, user.ID, "m2", base.Add(2*time.Minute))
	insertMessageAt(t, env, room.ID, user.ID, "m3", base.Add(3*time.Minute))

	page1, err := roomRepo.ListMessages(ctx, room.ID, base.Add(10*time.Minute), 2)
	if err != nil {
		t.Fatalf("list messages page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 messages page1, got %d", len(page1))
	}
	if page1[0].Content != "m3" || page1[1].Content != "m2" {
		t.Fatalf("unexpected page1 order: %q, %q", page1[0].Content, page1[1].Content)
	}

	page2, err := roomRepo.ListMessages(ctx, room.ID, page1[len(page1)-1].CreatedAt, 2)
	if err != nil {
		t.Fatalf("list messages page2: %v", err)
	}
	if len(page2) != 1 || page2[0].Content != "m1" {
		t.Fatalf("unexpected page2 result: len=%d first=%q", len(page2), firstContent(page2))
	}
}

func TestRepositoryDMHistoryIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	ctx := context.Background()
	if err := env.ResetDB(ctx); err != nil {
		t.Fatalf("reset db: %v", err)
	}

	userRepo := repository.NewUserRepo(env.Pool)
	dmRepo := repository.NewMessageRepo(env.Pool)

	u1 := &model.User{ID: uuid.New(), Username: "alice", Email: fmt.Sprintf("alice-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	u2 := &model.User{ID: uuid.New(), Username: "bob", Email: fmt.Sprintf("bob-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	if err := userRepo.Create(ctx, u1); err != nil {
		t.Fatalf("create user1: %v", err)
	}
	if err := userRepo.Create(ctx, u2); err != nil {
		t.Fatalf("create user2: %v", err)
	}

	insertDMAt(t, env, u1.ID, u2.ID, "a", time.Now().UTC().Add(-3*time.Minute))
	insertDMAt(t, env, u2.ID, u1.ID, "b", time.Now().UTC().Add(-2*time.Minute))
	insertDMAt(t, env, u1.ID, u2.ID, "c", time.Now().UTC().Add(-1*time.Minute))

	msgs, err := dmRepo.ListDMs(ctx, u1.ID, u2.ID, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("list dms: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 dms, got %d", len(msgs))
	}
	if msgs[0].Content != "c" || msgs[1].Content != "b" || msgs[2].Content != "a" {
		t.Fatalf("unexpected dm order: %q, %q, %q", msgs[0].Content, msgs[1].Content, msgs[2].Content)
	}
}

func TestRepositoryDMReceiptUpdatesIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	ctx := context.Background()
	if err := env.ResetDB(ctx); err != nil {
		t.Fatalf("reset db: %v", err)
	}

	userRepo := repository.NewUserRepo(env.Pool)
	dmRepo := repository.NewMessageRepo(env.Pool)

	sender := &model.User{ID: uuid.New(), Username: "sender", Email: fmt.Sprintf("sender-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	receiver := &model.User{ID: uuid.New(), Username: "receiver", Email: fmt.Sprintf("receiver-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	if err := userRepo.Create(ctx, sender); err != nil {
		t.Fatalf("create sender: %v", err)
	}
	if err := userRepo.Create(ctx, receiver); err != nil {
		t.Fatalf("create receiver: %v", err)
	}

	dmID := insertDMAtWithID(t, env, sender.ID, receiver.ID, "receipt", time.Now().UTC().Add(-1*time.Minute))

	firstDelivered := time.Now().UTC().Add(-30 * time.Second).Truncate(time.Second)
	if err := dmRepo.MarkDMDelivered(ctx, dmID, receiver.ID, firstDelivered); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}

	got, err := dmRepo.GetDMByID(ctx, dmID)
	if err != nil {
		t.Fatalf("get dm after delivered: %v", err)
	}
	if got.DeliveredAt == nil {
		t.Fatal("expected delivered_at to be set")
	}

	secondDelivered := firstDelivered.Add(20 * time.Second)
	if err := dmRepo.MarkDMDelivered(ctx, dmID, receiver.ID, secondDelivered); err != nil {
		t.Fatalf("mark delivered second time: %v", err)
	}

	got, err = dmRepo.GetDMByID(ctx, dmID)
	if err != nil {
		t.Fatalf("get dm second delivered check: %v", err)
	}
	if got.DeliveredAt == nil || !got.DeliveredAt.Equal(firstDelivered) {
		t.Fatalf("expected delivered_at to remain first value, got=%v want=%v", got.DeliveredAt, firstDelivered)
	}

	readAt := firstDelivered.Add(40 * time.Second)
	if err := dmRepo.MarkDMRead(ctx, dmID, receiver.ID, readAt); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	got, err = dmRepo.GetDMByID(ctx, dmID)
	if err != nil {
		t.Fatalf("get dm after read: %v", err)
	}
	if got.ReadAt == nil || !got.ReadAt.Equal(readAt) {
		t.Fatalf("expected read_at=%v, got=%v", readAt, got.ReadAt)
	}
	if got.DeliveredAt == nil || !got.DeliveredAt.Equal(firstDelivered) {
		t.Fatalf("expected delivered_at unchanged=%v, got=%v", firstDelivered, got.DeliveredAt)
	}
}

func TestRepositoryDMUnreadCountsIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	ctx := context.Background()
	if err := env.ResetDB(ctx); err != nil {
		t.Fatalf("reset db: %v", err)
	}

	userRepo := repository.NewUserRepo(env.Pool)
	dmRepo := repository.NewMessageRepo(env.Pool)

	owner := &model.User{ID: uuid.New(), Username: "owner", Email: fmt.Sprintf("owner-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	senderA := &model.User{ID: uuid.New(), Username: "sendera", Email: fmt.Sprintf("sendera-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	senderB := &model.User{ID: uuid.New(), Username: "senderb", Email: fmt.Sprintf("senderb-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	for _, u := range []*model.User{owner, senderA, senderB} {
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user %s: %v", u.Username, err)
		}
	}

	now := time.Now().UTC()
	dmA1 := insertDMAtWithID(t, env, senderA.ID, owner.ID, "a1", now.Add(-4*time.Minute))
	_ = insertDMAtWithID(t, env, senderA.ID, owner.ID, "a2", now.Add(-3*time.Minute))
	_ = insertDMAtWithID(t, env, senderB.ID, owner.ID, "b1", now.Add(-2*time.Minute))
	_ = insertDMAtWithID(t, env, owner.ID, senderA.ID, "outbound", now.Add(-1*time.Minute))

	if err := dmRepo.MarkDMRead(ctx, dmA1, owner.ID, now); err != nil {
		t.Fatalf("mark one dm as read: %v", err)
	}

	counts, err := dmRepo.ListDMUnreadCounts(ctx, owner.ID)
	if err != nil {
		t.Fatalf("list dm unread counts: %v", err)
	}
	if len(counts) != 2 {
		t.Fatalf("expected 2 conversation counts, got %d", len(counts))
	}

	got := map[uuid.UUID]int{}
	for _, c := range counts {
		got[c.UserID] = c.Count
	}
	if got[senderA.ID] != 1 {
		t.Fatalf("expected senderA unread=1, got %d", got[senderA.ID])
	}
	if got[senderB.ID] != 1 {
		t.Fatalf("expected senderB unread=1, got %d", got[senderB.ID])
	}
}

func TestRepositoryDMConversationHeadsIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	ctx := context.Background()
	if err := env.ResetDB(ctx); err != nil {
		t.Fatalf("reset db: %v", err)
	}

	userRepo := repository.NewUserRepo(env.Pool)
	dmRepo := repository.NewMessageRepo(env.Pool)

	owner := &model.User{ID: uuid.New(), Username: "owner-head", Email: fmt.Sprintf("owner-head-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	p1 := &model.User{ID: uuid.New(), Username: "p1", Email: fmt.Sprintf("p1-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	p2 := &model.User{ID: uuid.New(), Username: "p2", Email: fmt.Sprintf("p2-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	for _, u := range []*model.User{owner, p1, p2} {
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user %s: %v", u.Username, err)
		}
	}

	now := time.Now().UTC()
	insertDMAt(t, env, p1.ID, owner.ID, "p1-old", now.Add(-5*time.Minute))
	insertDMAt(t, env, owner.ID, p1.ID, "p1-latest", now.Add(-1*time.Minute))
	insertDMAt(t, env, p2.ID, owner.ID, "p2-latest", now.Add(-2*time.Minute))

	heads, err := dmRepo.ListDMConversationHeads(ctx, owner.ID, 50)
	if err != nil {
		t.Fatalf("list conversation heads: %v", err)
	}
	if len(heads) != 2 {
		t.Fatalf("expected 2 conversation heads, got %d", len(heads))
	}
	if heads[0].UserID != p1.ID || heads[0].LastMessage.Content != "p1-latest" {
		t.Fatalf("unexpected first head: %+v", heads[0])
	}
	if heads[1].UserID != p2.ID || heads[1].LastMessage.Content != "p2-latest" {
		t.Fatalf("unexpected second head: %+v", heads[1])
	}
}

func TestRepositoryRoomUnreadCountsIntegration(t *testing.T) {
	env := itest.Start(t, itest.EnvOptions{})
	ctx := context.Background()
	if err := env.ResetDB(ctx); err != nil {
		t.Fatalf("reset db: %v", err)
	}

	userRepo := repository.NewUserRepo(env.Pool)
	roomRepo := repository.NewRoomRepo(env.Pool)

	owner := &model.User{ID: uuid.New(), Username: "owner-ru", Email: fmt.Sprintf("owner-ru-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	u2 := &model.User{ID: uuid.New(), Username: "u2-ru", Email: fmt.Sprintf("u2-ru-%s@example.com", uuid.NewString()), PasswordHash: "hash"}
	for _, u := range []*model.User{owner, u2} {
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user %s: %v", u.Username, err)
		}
	}

	room := &model.Room{ID: uuid.New(), Name: "room-ru", Description: "desc", CreatedBy: owner.ID}
	if err := roomRepo.Create(ctx, room); err != nil {
		t.Fatalf("create room: %v", err)
	}
	if err := roomRepo.AddMember(ctx, room.ID, owner.ID); err != nil {
		t.Fatalf("add owner member: %v", err)
	}
	if err := roomRepo.AddMember(ctx, room.ID, u2.ID); err != nil {
		t.Fatalf("add u2 member: %v", err)
	}

	now := time.Now().UTC()
	if err := roomRepo.InitRoomReadState(ctx, room.ID, u2.ID, now); err != nil {
		t.Fatalf("init read state: %v", err)
	}

	insertMessageAt(t, env, room.ID, owner.ID, "m1", now.Add(1*time.Minute))
	insertMessageAt(t, env, room.ID, owner.ID, "m2", now.Add(2*time.Minute))
	insertMessageAt(t, env, room.ID, u2.ID, "self message", now.Add(3*time.Minute))

	counts, err := roomRepo.ListRoomUnreadCounts(ctx, u2.ID)
	if err != nil {
		t.Fatalf("list room unread counts: %v", err)
	}
	if len(counts) != 1 || counts[0].RoomID != room.ID || counts[0].Count != 2 {
		t.Fatalf("unexpected unread counts before read: %+v", counts)
	}

	if err := roomRepo.MarkRoomRead(ctx, room.ID, u2.ID, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("mark room read: %v", err)
	}
	countsAfter, err := roomRepo.ListRoomUnreadCounts(ctx, u2.ID)
	if err != nil {
		t.Fatalf("list room unread counts after read: %v", err)
	}
	if len(countsAfter) != 1 || countsAfter[0].Count != 0 {
		t.Fatalf("unexpected unread counts after read: %+v", countsAfter)
	}
}

func insertMessageAt(t *testing.T, env *itest.Env, roomID, senderID uuid.UUID, content string, createdAt time.Time) {
	t.Helper()
	_, err := env.Pool.Exec(context.Background(),
		`INSERT INTO messages (id, room_id, sender_id, content, created_at) VALUES ($1,$2,$3,$4,$5)`,
		uuid.New(), roomID, senderID, content, createdAt,
	)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
}

func insertDMAt(t *testing.T, env *itest.Env, senderID, receiverID uuid.UUID, content string, createdAt time.Time) {
	t.Helper()
	_, err := env.Pool.Exec(context.Background(),
		`INSERT INTO direct_messages (id, sender_id, receiver_id, content, created_at) VALUES ($1,$2,$3,$4,$5)`,
		uuid.New(), senderID, receiverID, content, createdAt,
	)
	if err != nil {
		t.Fatalf("insert dm: %v", err)
	}
}

func insertDMAtWithID(t *testing.T, env *itest.Env, senderID, receiverID uuid.UUID, content string, createdAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := env.Pool.Exec(context.Background(),
		`INSERT INTO direct_messages (id, sender_id, receiver_id, content, created_at) VALUES ($1,$2,$3,$4,$5)`,
		id, senderID, receiverID, content, createdAt,
	)
	if err != nil {
		t.Fatalf("insert dm with id: %v", err)
	}
	return id
}

func firstContent(msgs []*model.Message) string {
	if len(msgs) == 0 {
		return ""
	}
	return msgs[0].Content
}
