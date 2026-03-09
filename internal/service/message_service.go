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

type messageRoomRepository interface {
	IsMember(ctx context.Context, roomID, userID uuid.UUID) (bool, error)
	SaveMessage(ctx context.Context, m *model.Message) error
	ListMessages(ctx context.Context, roomID uuid.UUID, before time.Time, limit int) ([]*model.Message, error)
}

type messageRepository interface {
	SaveDM(ctx context.Context, m *model.DirectMessage) error
	ListDMs(ctx context.Context, userA, userB uuid.UUID, before time.Time, limit int) ([]*model.DirectMessage, error)
}

type eventPublisher interface {
	Publish(topic, key string, evt kafka.ChatEvent) error
}

func NewMessageService(rr messageRoomRepository, mr messageRepository, p eventPublisher) *MessageService {
	return &MessageService{roomRepo: rr, messageRepo: mr, producer: p}
}

func (s *MessageService) SendRoomMessage(ctx context.Context, roomID, senderID uuid.UUID, content string) (*model.Message, error) {
	ok, err := s.roomRepo.IsMember(ctx, roomID, senderID)
	if err != nil || !ok {
		return nil, errors.New("sender is not a room member")
	}

	msg := &model.Message{
		ID:        uuid.New(),
		RoomID:    roomID,
		SenderID:  senderID,
		Content:   content,
		CreatedAt: time.Now(),
	}
	if err := s.roomRepo.SaveMessage(ctx, msg); err != nil {
		return nil, fmt.Errorf("save room message: %w", err)
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
	dm := &model.DirectMessage{
		ID:         uuid.New(),
		SenderID:   senderID,
		ReceiverID: receiverID,
		Content:    content,
		CreatedAt:  time.Now(),
	}
	if err := s.messageRepo.SaveDM(ctx, dm); err != nil {
		return nil, fmt.Errorf("save dm: %w", err)
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
