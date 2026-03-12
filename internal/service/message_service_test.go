package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/kafka"
	"github.com/poyrazk/cloudtalk/internal/model"
)

type fakeMessageRoomRepo struct {
	isMember     bool
	isMemberErr  error
	saveErr      error
	getErr       error
	updateErr    error
	deleteErr    error
	listResp     []*model.Message
	listErr      error
	savedMessage *model.Message
	messageByID  *model.Message
}

func (f *fakeMessageRoomRepo) IsMember(_ context.Context, _, _ uuid.UUID) (bool, error) {
	if f.isMemberErr != nil {
		return false, f.isMemberErr
	}
	return f.isMember, nil
}

func (f *fakeMessageRoomRepo) SaveMessage(_ context.Context, m *model.Message) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.savedMessage = m
	return nil
}

func (f *fakeMessageRoomRepo) ListMessages(_ context.Context, _ uuid.UUID, _ time.Time, _ int) ([]*model.Message, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResp, nil
}

func (f *fakeMessageRoomRepo) GetMessageByID(_ context.Context, _ uuid.UUID) (*model.Message, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.messageByID == nil {
		return nil, errors.New("not found")
	}
	return f.messageByID, nil
}

func (f *fakeMessageRoomRepo) UpdateMessageContent(_ context.Context, _ uuid.UUID, content string, editedAt time.Time) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.messageByID != nil {
		f.messageByID.Content = content
		t := editedAt
		f.messageByID.EditedAt = &t
	}
	return nil
}

func (f *fakeMessageRoomRepo) SoftDeleteMessage(_ context.Context, _ uuid.UUID, deletedAt time.Time) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if f.messageByID != nil && f.messageByID.DeletedAt == nil {
		t := deletedAt
		f.messageByID.DeletedAt = &t
	}
	return nil
}

type fakeMessageRepo struct {
	saveErr          error
	listResp         []*model.DirectMessage
	listErr          error
	conversationResp []*model.DMConversationHead
	conversationErr  error
	unreadCountsResp []*model.DMUnreadCount
	unreadCountsErr  error
	savedDM          *model.DirectMessage
	getByIDErr       error
	markDeliveredErr error
	markReadErr      error
	updateErr        error
	deleteErr        error
	dmByID           *model.DirectMessage
}

type fakePresenceReader struct {
	online map[uuid.UUID]bool
}

func (f *fakePresenceReader) IsOnline(userID uuid.UUID) bool {
	return f.online[userID]
}

func (f *fakeMessageRepo) SaveDM(_ context.Context, m *model.DirectMessage) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.savedDM = m
	return nil
}

func (f *fakeMessageRepo) ListDMs(_ context.Context, _, _ uuid.UUID, _ time.Time, _ int) ([]*model.DirectMessage, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResp, nil
}

func (f *fakeMessageRepo) ListDMConversationHeads(_ context.Context, _ uuid.UUID, _ int) ([]*model.DMConversationHead, error) {
	if f.conversationErr != nil {
		return nil, f.conversationErr
	}
	return f.conversationResp, nil
}

func (f *fakeMessageRepo) ListDMUnreadCounts(_ context.Context, _ uuid.UUID) ([]*model.DMUnreadCount, error) {
	if f.unreadCountsErr != nil {
		return nil, f.unreadCountsErr
	}
	return f.unreadCountsResp, nil
}

func (f *fakeMessageRepo) GetDMByID(_ context.Context, _ uuid.UUID) (*model.DirectMessage, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	if f.dmByID == nil {
		return nil, errors.New("not found")
	}
	return f.dmByID, nil
}

func (f *fakeMessageRepo) MarkDMDelivered(_ context.Context, _ uuid.UUID, _ uuid.UUID, at time.Time) error {
	if f.markDeliveredErr != nil {
		return f.markDeliveredErr
	}
	if f.dmByID != nil && f.dmByID.DeliveredAt == nil {
		t := at
		f.dmByID.DeliveredAt = &t
	}
	return nil
}

func (f *fakeMessageRepo) MarkDMRead(_ context.Context, _ uuid.UUID, _ uuid.UUID, at time.Time) error {
	if f.markReadErr != nil {
		return f.markReadErr
	}
	if f.dmByID != nil {
		if f.dmByID.DeliveredAt == nil {
			t := at
			f.dmByID.DeliveredAt = &t
		}
		if f.dmByID.ReadAt == nil {
			t := at
			f.dmByID.ReadAt = &t
		}
	}
	return nil
}

func (f *fakeMessageRepo) UpdateDMContent(_ context.Context, _ uuid.UUID, content string, editedAt time.Time) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.dmByID != nil {
		f.dmByID.Content = content
		t := editedAt
		f.dmByID.EditedAt = &t
	}
	return nil
}

func (f *fakeMessageRepo) SoftDeleteDM(_ context.Context, _ uuid.UUID, deletedAt time.Time) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if f.dmByID != nil && f.dmByID.DeletedAt == nil {
		t := deletedAt
		f.dmByID.DeletedAt = &t
	}
	return nil
}

type fakePublisher struct {
	err      error
	topic    string
	key      string
	event    kafka.ChatEvent
	publishN int
}

type fakeMessageUserRepo struct {
	users map[uuid.UUID]*model.User
	err   error
}

func (f *fakeMessageUserRepo) GetByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	u, ok := f.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return u, nil
}

func (f *fakePublisher) Publish(topic, key string, evt kafka.ChatEvent) error {
	f.topic = topic
	f.key = key
	f.event = evt
	f.publishN++
	return f.err
}

func TestMessageServiceSendRoomMessageRequiresMembership(t *testing.T) {
	t.Parallel()

	rr := &fakeMessageRoomRepo{isMember: false}
	svc := NewMessageService(rr, &fakeMessageRepo{}, &fakeMessageUserRepo{}, &fakePublisher{})

	_, err := svc.SendRoomMessage(context.Background(), uuid.New(), uuid.New(), "hello")
	if !errors.Is(err, ErrRoomMembershipRequired) {
		t.Fatalf("expected ErrRoomMembershipRequired, got %v", err)
	}
}

func TestMessageServiceSendRoomMessageMembershipCheckFailure(t *testing.T) {
	t.Parallel()

	rr := &fakeMessageRoomRepo{isMemberErr: errors.New("db timeout")}
	svc := NewMessageService(rr, &fakeMessageRepo{}, &fakeMessageUserRepo{}, &fakePublisher{})

	_, err := svc.SendRoomMessage(context.Background(), uuid.New(), uuid.New(), "hello")
	if !errors.Is(err, ErrRoomMembershipCheck) {
		t.Fatalf("expected ErrRoomMembershipCheck, got %v", err)
	}
}

func TestMessageServiceSendRoomMessagePublishes(t *testing.T) {
	t.Parallel()

	rr := &fakeMessageRoomRepo{isMember: true}
	pub := &fakePublisher{}
	rid := uuid.New()
	sid := uuid.New()
	svc := NewMessageService(rr, &fakeMessageRepo{}, &fakeMessageUserRepo{}, pub)

	msg, err := svc.SendRoomMessage(context.Background(), rid, sid, "hello")
	if err != nil {
		t.Fatalf("send room message failed: %v", err)
	}
	if msg == nil || rr.savedMessage == nil {
		t.Fatal("expected message persisted")
	}
	if pub.publishN != 1 || pub.topic != kafka.TopicRoomMessages {
		t.Fatal("expected one room publish")
	}
}

func TestMessageServiceSendDMPublishFailureDoesNotFailCall(t *testing.T) {
	t.Parallel()

	mr := &fakeMessageRepo{}
	pub := &fakePublisher{err: errors.New("kafka down")}
	svc := NewMessageService(&fakeMessageRoomRepo{}, mr, &fakeMessageUserRepo{}, pub)

	dm, err := svc.SendDM(context.Background(), uuid.New(), uuid.New(), "ping")
	if err != nil {
		t.Fatalf("expected nil error when publish fails, got %v", err)
	}
	if dm == nil || mr.savedDM == nil {
		t.Fatal("expected dm persisted")
	}
}

func TestMessageServiceSendDMToSelfForbidden(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	svc := NewMessageService(&fakeMessageRoomRepo{}, &fakeMessageRepo{}, &fakeMessageUserRepo{}, &fakePublisher{})

	_, err := svc.SendDM(context.Background(), uid, uid, "ping")
	if !errors.Is(err, ErrDMToSelfForbidden) {
		t.Fatalf("expected ErrDMToSelfForbidden, got %v", err)
	}
}

func TestMessageServiceSendDMPersistenceError(t *testing.T) {
	t.Parallel()

	mr := &fakeMessageRepo{saveErr: errors.New("write failed")}
	svc := NewMessageService(&fakeMessageRoomRepo{}, mr, &fakeMessageUserRepo{}, &fakePublisher{})

	_, err := svc.SendDM(context.Background(), uuid.New(), uuid.New(), "ping")
	if !errors.Is(err, ErrMessagePersistence) {
		t.Fatalf("expected ErrMessagePersistence, got %v", err)
	}
}

func TestMessageServiceHistoryDelegation(t *testing.T) {
	t.Parallel()

	now := time.Now()
	rr := &fakeMessageRoomRepo{listResp: []*model.Message{{ID: uuid.New(), CreatedAt: now}}}
	mr := &fakeMessageRepo{listResp: []*model.DirectMessage{{ID: uuid.New(), CreatedAt: now}}}
	svc := NewMessageService(rr, mr, &fakeMessageUserRepo{}, &fakePublisher{})

	roomMsgs, err := svc.RoomHistory(context.Background(), uuid.New(), now, 20)
	if err != nil || len(roomMsgs) != 1 {
		t.Fatalf("room history delegation failed: err=%v len=%d", err, len(roomMsgs))
	}
	dmMsgs, err := svc.DMHistory(context.Background(), uuid.New(), uuid.New(), now, 20)
	if err != nil || len(dmMsgs) != 1 {
		t.Fatalf("dm history delegation failed: err=%v len=%d", err, len(dmMsgs))
	}
}

func TestMessageServiceDMUnreadCountsDelegation(t *testing.T) {
	t.Parallel()

	mr := &fakeMessageRepo{unreadCountsResp: []*model.DMUnreadCount{{UserID: uuid.New(), Count: 2}}}
	svc := NewMessageService(&fakeMessageRoomRepo{}, mr, &fakeMessageUserRepo{}, &fakePublisher{})

	counts, err := svc.DMUnreadCounts(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("dm unread counts failed: %v", err)
	}
	if len(counts) != 1 || counts[0].Count != 2 {
		t.Fatalf("unexpected unread counts result: %+v", counts)
	}
}

func TestMessageServiceMarkDMDelivered(t *testing.T) {
	t.Parallel()

	receiverID := uuid.New()
	dm := &model.DirectMessage{ID: uuid.New(), SenderID: uuid.New(), ReceiverID: receiverID}
	mr := &fakeMessageRepo{dmByID: dm}
	pub := &fakePublisher{}
	svc := NewMessageService(&fakeMessageRoomRepo{}, mr, &fakeMessageUserRepo{}, pub)

	updated, err := svc.MarkDMDelivered(context.Background(), dm.ID, receiverID)
	if err != nil {
		t.Fatalf("mark delivered failed: %v", err)
	}
	if updated.DeliveredAt == nil {
		t.Fatal("expected delivered_at to be set")
	}
	if pub.publishN == 0 || pub.event.Type != "dm_receipt" {
		t.Fatal("expected dm_receipt publish")
	}
}

func TestMessageServiceMarkDMReadForbiddenForSender(t *testing.T) {
	t.Parallel()

	dm := &model.DirectMessage{ID: uuid.New(), SenderID: uuid.New(), ReceiverID: uuid.New()}
	mr := &fakeMessageRepo{dmByID: dm}
	svc := NewMessageService(&fakeMessageRoomRepo{}, mr, &fakeMessageUserRepo{}, &fakePublisher{})

	_, err := svc.MarkDMRead(context.Background(), dm.ID, dm.SenderID)
	if !errors.Is(err, ErrDMReceiptForbidden) {
		t.Fatalf("expected ErrDMReceiptForbidden, got %v", err)
	}
}

func TestMessageServiceMarkDMReadSetsReadAndDelivered(t *testing.T) {
	t.Parallel()

	receiverID := uuid.New()
	dm := &model.DirectMessage{ID: uuid.New(), SenderID: uuid.New(), ReceiverID: receiverID}
	mr := &fakeMessageRepo{dmByID: dm}
	svc := NewMessageService(&fakeMessageRoomRepo{}, mr, &fakeMessageUserRepo{}, &fakePublisher{})

	updated, err := svc.MarkDMRead(context.Background(), dm.ID, receiverID)
	if err != nil {
		t.Fatalf("mark read failed: %v", err)
	}
	if updated.DeliveredAt == nil || updated.ReadAt == nil {
		t.Fatal("expected delivered_at and read_at to be set")
	}
}

func TestMessageServiceEditRoomMessage(t *testing.T) {
	t.Parallel()

	actor := uuid.New()
	m := &model.Message{ID: uuid.New(), SenderID: actor, Content: "old"}
	rr := &fakeMessageRoomRepo{messageByID: m}
	pub := &fakePublisher{}
	svc := NewMessageService(rr, &fakeMessageRepo{}, &fakeMessageUserRepo{}, pub)

	updated, err := svc.EditRoomMessage(context.Background(), m.ID, actor, "new")
	if err != nil {
		t.Fatalf("edit room message failed: %v", err)
	}
	if updated.EditedAt == nil || updated.Content != "new" {
		t.Fatal("expected edited room message")
	}
	if pub.event.Type != "message_updated" {
		t.Fatalf("expected message_updated event, got %q", pub.event.Type)
	}
}

func TestMessageServiceDeleteDMForbiddenForNonSender(t *testing.T) {
	t.Parallel()

	dm := &model.DirectMessage{ID: uuid.New(), SenderID: uuid.New(), ReceiverID: uuid.New()}
	mr := &fakeMessageRepo{dmByID: dm}
	svc := NewMessageService(&fakeMessageRoomRepo{}, mr, &fakeMessageUserRepo{}, &fakePublisher{})

	_, err := svc.DeleteDM(context.Background(), dm.ID, dm.ReceiverID)
	if !errors.Is(err, ErrMessageDeleteForbidden) {
		t.Fatalf("expected ErrMessageDeleteForbidden, got %v", err)
	}
}

func TestMessageServiceEditDMRejectedWhenDeleted(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	dm := &model.DirectMessage{ID: uuid.New(), SenderID: uuid.New(), DeletedAt: &now}
	mr := &fakeMessageRepo{dmByID: dm}
	svc := NewMessageService(&fakeMessageRoomRepo{}, mr, &fakeMessageUserRepo{}, &fakePublisher{})

	_, err := svc.EditDM(context.Background(), dm.ID, dm.SenderID, "updated")
	if !errors.Is(err, ErrMessageDeleted) {
		t.Fatalf("expected ErrMessageDeleted, got %v", err)
	}
}

func TestMessageServiceDMConversations(t *testing.T) {
	t.Parallel()

	partnerID := uuid.New()
	now := time.Now().UTC()
	offlineSeen := now.Add(-10 * time.Minute)
	mr := &fakeMessageRepo{
		conversationResp: []*model.DMConversationHead{{
			UserID: partnerID,
			LastMessage: &model.DirectMessage{
				ID:         uuid.New(),
				SenderID:   partnerID,
				ReceiverID: uuid.New(),
				Content:    "latest",
				CreatedAt:  now,
			},
		}},
		unreadCountsResp: []*model.DMUnreadCount{{UserID: partnerID, Count: 4}},
	}
	ur := &fakeMessageUserRepo{users: map[uuid.UUID]*model.User{partnerID: {ID: partnerID, Username: "alice", LastSeenAt: &offlineSeen}}}
	pr := &fakePresenceReader{online: map[uuid.UUID]bool{partnerID: true}}
	svc := NewMessageServiceWithPresence(&fakeMessageRoomRepo{}, mr, ur, &fakePublisher{}, pr)

	convs, err := svc.DMConversations(context.Background(), uuid.New(), 50)
	if err != nil {
		t.Fatalf("dm conversations failed: %v", err)
	}
	if len(convs) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(convs))
	}
	if convs[0].Username != "alice" || convs[0].UnreadCount != 4 || !convs[0].Online || convs[0].LastSeen != nil {
		t.Fatalf("unexpected conversation projection: %+v", convs[0])
	}

	pr.online[partnerID] = false
	convs, err = svc.DMConversations(context.Background(), uuid.New(), 50)
	if err != nil {
		t.Fatalf("dm conversations failed: %v", err)
	}
	if len(convs) != 1 || convs[0].Online || convs[0].LastSeen == nil {
		t.Fatalf("expected offline last_seen projection, got %+v", convs[0])
	}
}
