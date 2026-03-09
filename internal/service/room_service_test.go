package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/model"
)

type fakeRoomRepo struct {
	createErr      error
	getErr         error
	addMemberErr   error
	removeErr      error
	isMember       bool
	isMemberErr    error
	createdRoom    *model.Room
	addedRoomID    uuid.UUID
	addedUserID    uuid.UUID
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
