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
	userRepo    messageUserRepository
	producer    eventPublisher
}

var (
	ErrRoomMembershipRequired = errors.New("room membership required")
	ErrRoomMembershipCheck    = errors.New("room membership check failed")
	ErrDMToSelfForbidden      = errors.New("cannot send dm to yourself")
	ErrMessagePersistence     = errors.New("message persistence failed")
	ErrDMNotFound             = errors.New("dm not found")
	ErrDMReceiptForbidden     = errors.New("dm receipt forbidden")
	ErrMessageNotFound        = errors.New("message not found")
	ErrMessageEditForbidden   = errors.New("message edit forbidden")
	ErrMessageDeleteForbidden = errors.New("message delete forbidden")
	ErrMessageDeleted         = errors.New("message is deleted")
)

type messageRoomRepository interface {
	IsMember(ctx context.Context, roomID, userID uuid.UUID) (bool, error)
	SaveMessage(ctx context.Context, m *model.Message) error
	ListMessages(ctx context.Context, roomID uuid.UUID, before time.Time, limit int) ([]*model.Message, error)
	GetMessageByID(ctx context.Context, id uuid.UUID) (*model.Message, error)
	UpdateMessageContent(ctx context.Context, id uuid.UUID, content string, editedAt time.Time) error
	SoftDeleteMessage(ctx context.Context, id uuid.UUID, deletedAt time.Time) error
}

type messageRepository interface {
	SaveDM(ctx context.Context, m *model.DirectMessage) error
	ListDMUnreadCounts(ctx context.Context, userID uuid.UUID) ([]*model.DMUnreadCount, error)
	ListDMConversationHeads(ctx context.Context, userID uuid.UUID, limit int) ([]*model.DMConversationHead, error)
	ListDMs(ctx context.Context, userA, userB uuid.UUID, before time.Time, limit int) ([]*model.DirectMessage, error)
	GetDMByID(ctx context.Context, id uuid.UUID) (*model.DirectMessage, error)
	MarkDMDelivered(ctx context.Context, dmID, receiverID uuid.UUID, at time.Time) error
	MarkDMRead(ctx context.Context, dmID, receiverID uuid.UUID, at time.Time) error
	UpdateDMContent(ctx context.Context, id uuid.UUID, content string, editedAt time.Time) error
	SoftDeleteDM(ctx context.Context, id uuid.UUID, deletedAt time.Time) error
}

type messageUserRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.User, error)
}

type eventPublisher interface {
	Publish(topic, key string, evt kafka.ChatEvent) error
}

func NewMessageService(rr messageRoomRepository, mr messageRepository, ur messageUserRepository, p eventPublisher) *MessageService {
	return &MessageService{roomRepo: rr, messageRepo: mr, userRepo: ur, producer: p}
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

func (s *MessageService) DMConversations(ctx context.Context, userID uuid.UUID, limit int) ([]*model.DMConversation, error) {
	heads, err := s.messageRepo.ListDMConversationHeads(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("load dm conversation heads: %w", err)
	}
	counts, err := s.messageRepo.ListDMUnreadCounts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load dm unread counts for conversations: %w", err)
	}
	countMap := make(map[uuid.UUID]int, len(counts))
	for _, c := range counts {
		countMap[c.UserID] = c.Count
	}

	convs := make([]*model.DMConversation, 0, len(heads))
	for _, head := range heads {
		user, err := s.userRepo.GetByID(ctx, head.UserID)
		if err != nil {
			return nil, fmt.Errorf("load conversation user: %w", err)
		}
		convs = append(convs, &model.DMConversation{
			UserID:      user.ID,
			Username:    user.Username,
			UnreadCount: countMap[user.ID],
			LastMessage: head.LastMessage,
		})
	}

	return convs, nil
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

func (s *MessageService) EditRoomMessage(ctx context.Context, messageID, actorID uuid.UUID, newContent string) (*model.Message, error) {
	msg, err := s.roomRepo.GetMessageByID(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMessageNotFound, err)
	}
	if msg.SenderID != actorID {
		return nil, ErrMessageEditForbidden
	}
	if msg.DeletedAt != nil {
		return nil, ErrMessageDeleted
	}
	now := time.Now().UTC()
	if err := s.roomRepo.UpdateMessageContent(ctx, messageID, newContent, now); err != nil {
		return nil, fmt.Errorf("%w: edit room message: %w", ErrMessagePersistence, err)
	}
	updated, err := s.roomRepo.GetMessageByID(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("%w: reload edited room message: %w", ErrMessagePersistence, err)
	}
	s.publishRoomEvent("message_updated", updated)
	return updated, nil
}

func (s *MessageService) DeleteRoomMessage(ctx context.Context, messageID, actorID uuid.UUID) (*model.Message, error) {
	msg, err := s.roomRepo.GetMessageByID(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMessageNotFound, err)
	}
	if msg.SenderID != actorID {
		return nil, ErrMessageDeleteForbidden
	}
	now := time.Now().UTC()
	if err := s.roomRepo.SoftDeleteMessage(ctx, messageID, now); err != nil {
		return nil, fmt.Errorf("%w: delete room message: %w", ErrMessagePersistence, err)
	}
	updated, err := s.roomRepo.GetMessageByID(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("%w: reload deleted room message: %w", ErrMessagePersistence, err)
	}
	s.publishRoomEvent("message_deleted", updated)
	return updated, nil
}

func (s *MessageService) EditDM(ctx context.Context, dmID, actorID uuid.UUID, newContent string) (*model.DirectMessage, error) {
	dm, err := s.messageRepo.GetDMByID(ctx, dmID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDMNotFound, err)
	}
	if dm.SenderID != actorID {
		return nil, ErrMessageEditForbidden
	}
	if dm.DeletedAt != nil {
		return nil, ErrMessageDeleted
	}
	now := time.Now().UTC()
	if err := s.messageRepo.UpdateDMContent(ctx, dmID, newContent, now); err != nil {
		return nil, fmt.Errorf("%w: edit dm: %w", ErrMessagePersistence, err)
	}
	updated, err := s.messageRepo.GetDMByID(ctx, dmID)
	if err != nil {
		return nil, fmt.Errorf("%w: reload edited dm: %w", ErrMessagePersistence, err)
	}
	s.publishDMEvent("dm_updated", updated)
	return updated, nil
}

func (s *MessageService) DeleteDM(ctx context.Context, dmID, actorID uuid.UUID) (*model.DirectMessage, error) {
	dm, err := s.messageRepo.GetDMByID(ctx, dmID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDMNotFound, err)
	}
	if dm.SenderID != actorID {
		return nil, ErrMessageDeleteForbidden
	}
	now := time.Now().UTC()
	if err := s.messageRepo.SoftDeleteDM(ctx, dmID, now); err != nil {
		return nil, fmt.Errorf("%w: delete dm: %w", ErrMessagePersistence, err)
	}
	updated, err := s.messageRepo.GetDMByID(ctx, dmID)
	if err != nil {
		return nil, fmt.Errorf("%w: reload deleted dm: %w", ErrMessagePersistence, err)
	}
	s.publishDMEvent("dm_deleted", updated)
	return updated, nil
}

func (s *MessageService) publishRoomEvent(eventType string, msg *model.Message) {
	payload, err := json.Marshal(msg)
	if err != nil {
		slog.Error("marshal room event", "type", eventType, "err", err)
		return
	}
	if err := s.producer.Publish(kafka.TopicRoomMessages, msg.RoomID.String(), kafka.ChatEvent{
		Type:     eventType,
		RoomID:   msg.RoomID.String(),
		SenderID: msg.SenderID.String(),
		Payload:  payload,
	}); err != nil {
		slog.Error("publish room event", "type", eventType, "err", err)
	}
}

func (s *MessageService) publishDMEvent(eventType string, dm *model.DirectMessage) {
	payload, err := json.Marshal(dm)
	if err != nil {
		slog.Error("marshal dm event", "type", eventType, "err", err)
		return
	}
	if err := s.producer.Publish(kafka.TopicDMMessages, dm.ReceiverID.String(), kafka.ChatEvent{
		Type:     eventType,
		SenderID: dm.SenderID.String(),
		ToUserID: dm.ReceiverID.String(),
		Payload:  payload,
	}); err != nil {
		slog.Error("publish dm event", "type", eventType, "err", err)
	}
}
