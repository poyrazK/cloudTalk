package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/kafka"
	"github.com/poyrazk/cloudtalk/internal/model"
)

type MessageService struct {
	roomRepo    messageRoomRepository
	messageRepo messageRepository
	producer    eventPublisher
}

var (
	ErrRoomMembershipRequired = errors.New("room membership required")
	ErrRoomMembershipCheck    = errors.New("room membership check failed")
	ErrDMToSelfForbidden      = errors.New("cannot send dm to yourself")
	ErrMessagePersistence     = errors.New("message persistence failed")
	ErrDMNotFound             = errors.New("dm not found")
	ErrDMReceiptForbidden     = errors.New("dm receipt forbidden")
)

type messageRoomRepository interface {
	IsMember(ctx context.Context, roomID, userID uuid.UUID) (bool, error)
	SaveMessage(ctx context.Context, m *model.Message) error
	ListMessages(ctx context.Context, roomID uuid.UUID, before time.Time, limit int) ([]*model.Message, error)
}

type messageRepository interface {
	SaveDM(ctx context.Context, m *model.DirectMessage) error
	ListDMUnreadCounts(ctx context.Context, userID uuid.UUID) ([]*model.DMUnreadCount, error)
	ListDMs(ctx context.Context, userA, userB uuid.UUID, before time.Time, limit int) ([]*model.DirectMessage, error)
	GetDMByID(ctx context.Context, id uuid.UUID) (*model.DirectMessage, error)
	MarkDMDelivered(ctx context.Context, dmID, receiverID uuid.UUID, at time.Time) error
	MarkDMRead(ctx context.Context, dmID, receiverID uuid.UUID, at time.Time) error
}

type eventPublisher interface {
	Publish(topic, key string, evt kafka.ChatEvent) error
}

func NewMessageService(rr messageRoomRepository, mr messageRepository, p eventPublisher) *MessageService {
	return &MessageService{roomRepo: rr, messageRepo: mr, producer: p}
}

func (s *MessageService) SendRoomMessage(ctx context.Context, roomID, senderID uuid.UUID, content string) (*model.Message, error) {
	ok, err := s.roomRepo.IsMember(ctx, roomID, senderID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRoomMembershipCheck, err)
	}
	if !ok {
		return nil, ErrRoomMembershipRequired
	}

	msg := &model.Message{
		ID:        uuid.New(),
		RoomID:    roomID,
		SenderID:  senderID,
		Content:   content,
		CreatedAt: time.Now(),
	}
	if err := s.roomRepo.SaveMessage(ctx, msg); err != nil {
		return nil, fmt.Errorf("%w: save room message: %w", ErrMessagePersistence, err)
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		slog.Error("marshal room message", "err", err)
		return msg, nil
	}
	if err := s.producer.Publish(kafka.TopicRoomMessages, roomID.String(), kafka.ChatEvent{
		Type:     "message",
		RoomID:   roomID.String(),
		SenderID: senderID.String(),
		Payload:  payload,
	}); err != nil {
		slog.Error("publish room message to kafka", "err", err)
	}
	return msg, nil
}

func (s *MessageService) SendDM(ctx context.Context, senderID, receiverID uuid.UUID, content string) (*model.DirectMessage, error) {
	if senderID == receiverID {
		return nil, ErrDMToSelfForbidden
	}

	dm := &model.DirectMessage{
		ID:         uuid.New(),
		SenderID:   senderID,
		ReceiverID: receiverID,
		Content:    content,
		CreatedAt:  time.Now(),
	}
	if err := s.messageRepo.SaveDM(ctx, dm); err != nil {
		return nil, fmt.Errorf("%w: save dm: %w", ErrMessagePersistence, err)
	}

	payload, err := json.Marshal(dm)
	if err != nil {
		slog.Error("marshal dm", "err", err)
		return dm, nil
	}
	if err := s.producer.Publish(kafka.TopicDMMessages, receiverID.String(), kafka.ChatEvent{
		Type:     "dm",
		SenderID: senderID.String(),
		ToUserID: receiverID.String(),
		Payload:  payload,
	}); err != nil {
		slog.Error("publish dm to kafka", "err", err)
	}
	return dm, nil
}

func (s *MessageService) RoomHistory(ctx context.Context, roomID uuid.UUID, before time.Time, limit int) ([]*model.Message, error) {
	msgs, err := s.roomRepo.ListMessages(ctx, roomID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("load room history: %w", err)
	}
	return msgs, nil
}

func (s *MessageService) DMHistory(ctx context.Context, userA, userB uuid.UUID, before time.Time, limit int) ([]*model.DirectMessage, error) {
	msgs, err := s.messageRepo.ListDMs(ctx, userA, userB, before, limit)
	if err != nil {
		return nil, fmt.Errorf("load dm history: %w", err)
	}
	return msgs, nil
}

func (s *MessageService) DMUnreadCounts(ctx context.Context, userID uuid.UUID) ([]*model.DMUnreadCount, error) {
	counts, err := s.messageRepo.ListDMUnreadCounts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load dm unread counts: %w", err)
	}
	return counts, nil
}

func (s *MessageService) MarkDMDelivered(ctx context.Context, dmID, actorID uuid.UUID) (*model.DirectMessage, error) {
	dm, err := s.messageRepo.GetDMByID(ctx, dmID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDMNotFound, err)
	}
	if dm.ReceiverID != actorID {
		return nil, ErrDMReceiptForbidden
	}
	now := time.Now().UTC()
	if err := s.messageRepo.MarkDMDelivered(ctx, dmID, actorID, now); err != nil {
		return nil, fmt.Errorf("%w: mark dm delivered: %w", ErrMessagePersistence, err)
	}
	updated, err := s.messageRepo.GetDMByID(ctx, dmID)
	if err != nil {
		return nil, fmt.Errorf("%w: reload dm after delivered: %w", ErrMessagePersistence, err)
	}
	s.publishDMReceipt(updated)
	return updated, nil
}

func (s *MessageService) MarkDMRead(ctx context.Context, dmID, actorID uuid.UUID) (*model.DirectMessage, error) {
	dm, err := s.messageRepo.GetDMByID(ctx, dmID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDMNotFound, err)
	}
	if dm.ReceiverID != actorID {
		return nil, ErrDMReceiptForbidden
	}
	now := time.Now().UTC()
	if err := s.messageRepo.MarkDMRead(ctx, dmID, actorID, now); err != nil {
		return nil, fmt.Errorf("%w: mark dm read: %w", ErrMessagePersistence, err)
	}
	updated, err := s.messageRepo.GetDMByID(ctx, dmID)
	if err != nil {
		return nil, fmt.Errorf("%w: reload dm after read: %w", ErrMessagePersistence, err)
	}
	s.publishDMReceipt(updated)
	return updated, nil
}

func (s *MessageService) publishDMReceipt(dm *model.DirectMessage) {
	payload, err := json.Marshal(map[string]interface{}{
		"dm_id":        dm.ID.String(),
		"sender_id":    dm.SenderID.String(),
		"receiver_id":  dm.ReceiverID.String(),
		"delivered_at": dm.DeliveredAt,
		"read_at":      dm.ReadAt,
	})
	if err != nil {
		slog.Error("marshal dm receipt", "err", err)
		return
	}
	if err := s.producer.Publish(kafka.TopicDMMessages, dm.ReceiverID.String(), kafka.ChatEvent{
		Type:     "dm_receipt",
		SenderID: dm.SenderID.String(),
		ToUserID: dm.ReceiverID.String(),
		Payload:  payload,
	}); err != nil {
		slog.Error("publish dm receipt", "err", err)
	}
}
