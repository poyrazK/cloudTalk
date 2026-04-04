package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/model"
	apptrace "github.com/poyrazk/cloudtalk/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
)

type RoomService struct {
	rooms    roomRepository
	presence roomPresenceReader
}

type roomRepository interface {
	Create(ctx context.Context, room *model.Room) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Room, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*model.Room, error)
	AddMember(ctx context.Context, roomID, userID uuid.UUID) error
	AddMemberWithRole(ctx context.Context, roomID, userID uuid.UUID, role string) error
	InitRoomReadState(ctx context.Context, roomID, userID uuid.UUID, at time.Time) error
	MarkRoomRead(ctx context.Context, roomID, userID uuid.UUID, at time.Time) error
	ListRoomUnreadCounts(ctx context.Context, userID uuid.UUID) ([]*model.RoomUnreadCount, error)
	ListRoomConversationHeads(ctx context.Context, userID uuid.UUID, limit int) ([]*model.RoomConversationHead, error)
	ListRoomMemberIDs(ctx context.Context, roomIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	ListRoomMembers(ctx context.Context, roomID uuid.UUID) ([]*model.RoomMemberDetail, error)
	GetMemberRole(ctx context.Context, roomID, userID uuid.UUID) (string, error)
	RemoveMember(ctx context.Context, roomID, userID uuid.UUID) error
	IsMember(ctx context.Context, roomID, userID uuid.UUID) (bool, error)
}

type roomPresenceReader interface {
	IsOnline(userID uuid.UUID) bool
}

func NewRoomService(rooms roomRepository) *RoomService {
	return NewRoomServiceWithPresence(rooms, nil)
}

func NewRoomServiceWithPresence(rooms roomRepository, presence roomPresenceReader) *RoomService {
	return &RoomService{rooms: rooms, presence: presence}
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
	_ = s.rooms.AddMemberWithRole(ctx, room.ID, createdBy, model.RoomRoleOwner)
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
		return errors.New("forbidden: not a room member")
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

func (s *RoomService) Conversations(ctx context.Context, userID uuid.UUID, limit int) ([]*model.RoomConversation, error) {
	ctx, span := apptrace.Tracer("cloudtalk/room_service").Start(ctx, "room.conversations")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID.String()), attribute.Int("pagination.limit", limit))

	heads, err := s.rooms.ListRoomConversationHeads(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list room conversation heads: %w", err)
	}

	counts, err := s.rooms.ListRoomUnreadCounts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list room unread counts for conversations: %w", err)
	}
	countByRoomID := make(map[uuid.UUID]int, len(counts))
	for _, c := range counts {
		countByRoomID[c.RoomID] = c.Count
	}
	roomIDs := make([]uuid.UUID, 0, len(heads))
	for _, head := range heads {
		roomIDs = append(roomIDs, head.RoomID)
	}

	membersByRoomID, err := s.rooms.ListRoomMemberIDs(ctx, roomIDs)
	if err != nil {
		return nil, fmt.Errorf("list room members for conversations: %w", err)
	}

	conversations := make([]*model.RoomConversation, 0, len(heads))
	for _, head := range heads {
		memberCount := len(membersByRoomID[head.RoomID])
		onlineCount := 0
		if s.presence != nil {
			for _, memberID := range membersByRoomID[head.RoomID] {
				if s.presence.IsOnline(memberID) {
					onlineCount++
				}
			}
		}
		conversations = append(conversations, &model.RoomConversation{
			RoomID:      head.RoomID,
			Name:        head.Name,
			Description: head.Description,
			MemberCount: memberCount,
			OnlineCount: onlineCount,
			UnreadCount: countByRoomID[head.RoomID],
			LastMessage: head.LastMessage,
		})
	}

	return conversations, nil
}

func (s *RoomService) Members(ctx context.Context, roomID uuid.UUID) ([]*model.RoomMemberDetail, error) {
	ctx, span := apptrace.Tracer("cloudtalk/room_service").Start(ctx, "room.members")
	defer span.End()
	span.SetAttributes(attribute.String("room.id", roomID.String()))

	members, err := s.rooms.ListRoomMembers(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("list room members: %w", err)
	}
	for _, member := range members {
		member.Online = s.presence != nil && s.presence.IsOnline(member.UserID)
		if member.Online {
			member.LastSeen = nil
		}
	}
	return members, nil
}

func (s *RoomService) RemoveMemberAsOwner(ctx context.Context, roomID, actorID, targetUserID uuid.UUID) error {
	if actorID == targetUserID {
		return errors.New("bad request: use leave to remove yourself from a room")
	}
	actorRole, err := s.rooms.GetMemberRole(ctx, roomID, actorID)
	if err != nil {
		return fmt.Errorf("load actor role: %w", err)
	}
	if actorRole != model.RoomRoleOwner {
		return errors.New("forbidden: only room owner can remove members")
	}
	targetRole, err := s.rooms.GetMemberRole(ctx, roomID, targetUserID)
	if err != nil {
		return fmt.Errorf("load target role: %w", err)
	}
	if targetRole == model.RoomRoleOwner {
		return errors.New("bad request: room owner cannot be removed")
	}
	if err := s.rooms.RemoveMember(ctx, roomID, targetUserID); err != nil {
		return fmt.Errorf("remove room member: %w", err)
	}
	return nil
}
