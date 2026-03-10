package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/model"
)

type RoomService struct {
	rooms roomRepository
}

type roomRepository interface {
	Create(ctx context.Context, room *model.Room) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Room, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*model.Room, error)
	AddMember(ctx context.Context, roomID, userID uuid.UUID) error
	InitRoomReadState(ctx context.Context, roomID, userID uuid.UUID, at time.Time) error
	MarkRoomRead(ctx context.Context, roomID, userID uuid.UUID, at time.Time) error
	ListRoomUnreadCounts(ctx context.Context, userID uuid.UUID) ([]*model.RoomUnreadCount, error)
	RemoveMember(ctx context.Context, roomID, userID uuid.UUID) error
	IsMember(ctx context.Context, roomID, userID uuid.UUID) (bool, error)
}

func NewRoomService(rooms roomRepository) *RoomService {
	return &RoomService{rooms: rooms}
}

func (s *RoomService) Create(ctx context.Context, name, description string, createdBy uuid.UUID) (*model.Room, error) {
	room := &model.Room{
		ID:          uuid.New(),
		Name:        name,
		Description: description,
		CreatedBy:   createdBy,
	}
	if err := s.rooms.Create(ctx, room); err != nil {
		return nil, fmt.Errorf("create room: %w", err)
	}
	// Creator automatically joins
	_ = s.rooms.AddMember(ctx, room.ID, createdBy)
	_ = s.rooms.InitRoomReadState(ctx, room.ID, createdBy, time.Now().UTC())
	return room, nil
}

func (s *RoomService) Get(ctx context.Context, id uuid.UUID) (*model.Room, error) {
	room, err := s.rooms.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get room: %w", err)
	}
	return room, nil
}

func (s *RoomService) ListByUser(ctx context.Context, userID uuid.UUID) ([]*model.Room, error) {
	rooms, err := s.rooms.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user rooms: %w", err)
	}
	return rooms, nil
}

func (s *RoomService) Join(ctx context.Context, roomID, userID uuid.UUID) error {
	if _, err := s.rooms.GetByID(ctx, roomID); err != nil {
		return fmt.Errorf("room not found: %w", err)
	}
	if err := s.rooms.AddMember(ctx, roomID, userID); err != nil {
		return fmt.Errorf("join room: %w", err)
	}
	if err := s.rooms.InitRoomReadState(ctx, roomID, userID, time.Now().UTC()); err != nil {
		return fmt.Errorf("init room read state: %w", err)
	}
	return nil
}

func (s *RoomService) Leave(ctx context.Context, roomID, userID uuid.UUID) error {
	if err := s.rooms.RemoveMember(ctx, roomID, userID); err != nil {
		return fmt.Errorf("leave room: %w", err)
	}
	return nil
}

func (s *RoomService) IsMember(ctx context.Context, roomID, userID uuid.UUID) (bool, error) {
	ok, err := s.rooms.IsMember(ctx, roomID, userID)
	if err != nil {
		return false, fmt.Errorf("check room member: %w", err)
	}
	return ok, nil
}

func (s *RoomService) MarkRead(ctx context.Context, roomID, userID uuid.UUID) error {
	ok, err := s.rooms.IsMember(ctx, roomID, userID)
	if err != nil {
		return fmt.Errorf("check room member: %w", err)
	}
	if !ok {
		return fmt.Errorf("forbidden: not a room member")
	}
	if err := s.rooms.MarkRoomRead(ctx, roomID, userID, time.Now().UTC()); err != nil {
		return fmt.Errorf("mark room read: %w", err)
	}
	return nil
}

func (s *RoomService) UnreadCounts(ctx context.Context, userID uuid.UUID) ([]*model.RoomUnreadCount, error) {
	counts, err := s.rooms.ListRoomUnreadCounts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list room unread counts: %w", err)
	}
	return counts, nil
}
