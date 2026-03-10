package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/model"
)

type fakeRoomRepo struct {
	createErr      error
	getErr         error
	addMemberErr   error
	initReadErr    error
	markReadErr    error
	removeErr      error
	isMember       bool
	isMemberErr    error
	createdRoom    *model.Room
	addedRoomID    uuid.UUID
	addedUserID    uuid.UUID
	initReadRoomID uuid.UUID
	initReadUserID uuid.UUID
	unreadResp     []*model.RoomUnreadCount
	removedRoomID  uuid.UUID
	removedUserID  uuid.UUID
	listByUserResp []*model.Room
}

func (f *fakeRoomRepo) Create(_ context.Context, room *model.Room) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createdRoom = room
	return nil
}

func (f *fakeRoomRepo) GetByID(_ context.Context, id uuid.UUID) (*model.Room, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &model.Room{ID: id}, nil
}

func (f *fakeRoomRepo) ListByUser(_ context.Context, _ uuid.UUID) ([]*model.Room, error) {
	return f.listByUserResp, nil
}

func (f *fakeRoomRepo) AddMember(_ context.Context, roomID, userID uuid.UUID) error {
	if f.addMemberErr != nil {
		return f.addMemberErr
	}
	f.addedRoomID = roomID
	f.addedUserID = userID
	return nil
}

func (f *fakeRoomRepo) InitRoomReadState(_ context.Context, roomID, userID uuid.UUID, _ time.Time) error {
	if f.initReadErr != nil {
		return f.initReadErr
	}
	f.initReadRoomID = roomID
	f.initReadUserID = userID
	return nil
}

func (f *fakeRoomRepo) MarkRoomRead(_ context.Context, _, _ uuid.UUID, _ time.Time) error {
	if f.markReadErr != nil {
		return f.markReadErr
	}
	return nil
}

func (f *fakeRoomRepo) ListRoomUnreadCounts(_ context.Context, _ uuid.UUID) ([]*model.RoomUnreadCount, error) {
	return f.unreadResp, nil
}

func (f *fakeRoomRepo) RemoveMember(_ context.Context, roomID, userID uuid.UUID) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removedRoomID = roomID
	f.removedUserID = userID
	return nil
}

func (f *fakeRoomRepo) IsMember(_ context.Context, _, _ uuid.UUID) (bool, error) {
	if f.isMemberErr != nil {
		return false, f.isMemberErr
	}
	return f.isMember, nil
}

func TestRoomServiceCreateAutoAddsCreator(t *testing.T) {
	t.Parallel()

	repo := &fakeRoomRepo{}
	svc := NewRoomService(repo)
	uid := uuid.New()

	room, err := svc.Create(context.Background(), "general", "desc", uid)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if room.ID == uuid.Nil {
		t.Fatal("expected generated room ID")
	}
	if repo.addedRoomID != room.ID || repo.addedUserID != uid {
		t.Fatal("expected creator to be auto-added")
	}
	if repo.initReadRoomID != room.ID || repo.initReadUserID != uid {
		t.Fatal("expected creator read state to be initialized")
	}
}

func TestRoomServiceJoinRoomNotFound(t *testing.T) {
	t.Parallel()

	repo := &fakeRoomRepo{getErr: errors.New("missing")}
	svc := NewRoomService(repo)
	err := svc.Join(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected join error")
	}
}

func TestRoomServiceMarkReadRequiresMembership(t *testing.T) {
	t.Parallel()

	repo := &fakeRoomRepo{isMember: false}
	svc := NewRoomService(repo)
	err := svc.MarkRead(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected forbidden mark read error")
	}
}

func TestRoomServiceUnreadCountsDelegation(t *testing.T) {
	t.Parallel()

	roomID := uuid.New()
	repo := &fakeRoomRepo{unreadResp: []*model.RoomUnreadCount{{RoomID: roomID, Count: 3}}}
	svc := NewRoomService(repo)

	counts, err := svc.UnreadCounts(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unread counts failed: %v", err)
	}
	if len(counts) != 1 || counts[0].RoomID != roomID || counts[0].Count != 3 {
		t.Fatalf("unexpected unread counts: %+v", counts)
	}
}

func TestRoomServiceLeaveDelegates(t *testing.T) {
	t.Parallel()

	repo := &fakeRoomRepo{}
	svc := NewRoomService(repo)
	rid := uuid.New()
	uid := uuid.New()

	if err := svc.Leave(context.Background(), rid, uid); err != nil {
		t.Fatalf("leave failed: %v", err)
	}
	if repo.removedRoomID != rid || repo.removedUserID != uid {
		t.Fatal("expected leave delegation")
	}
}
