package service

import (
	"context"
	"fmt"

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
	return room, nil
}

func (s *RoomService) Get(ctx context.Context, id uuid.UUID) (*model.Room, error) {
	return s.rooms.GetByID(ctx, id)
}

func (s *RoomService) ListByUser(ctx context.Context, userID uuid.UUID) ([]*model.Room, error) {
	return s.rooms.ListByUser(ctx, userID)
}

func (s *RoomService) Join(ctx context.Context, roomID, userID uuid.UUID) error {
	if _, err := s.rooms.GetByID(ctx, roomID); err != nil {
		return fmt.Errorf("room not found: %w", err)
	}
	return s.rooms.AddMember(ctx, roomID, userID)
}

func (s *RoomService) Leave(ctx context.Context, roomID, userID uuid.UUID) error {
	return s.rooms.RemoveMember(ctx, roomID, userID)
}

func (s *RoomService) IsMember(ctx context.Context, roomID, userID uuid.UUID) (bool, error) {
	return s.rooms.IsMember(ctx, roomID, userID)
}
