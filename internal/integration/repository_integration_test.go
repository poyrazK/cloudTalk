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

func firstContent(msgs []*model.Message) string {
	if len(msgs) == 0 {
		return ""
	}
	return msgs[0].Content
}
