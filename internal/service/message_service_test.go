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
	listResp     []*model.Message
	listErr      error
	savedMessage *model.Message
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

type fakeMessageRepo struct {
	saveErr  error
	listResp []*model.DirectMessage
	listErr  error
	savedDM  *model.DirectMessage
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

type fakePublisher struct {
	err      error
	topic    string
	key      string
	event    kafka.ChatEvent
	publishN int
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
	svc := NewMessageService(rr, &fakeMessageRepo{}, &fakePublisher{})

	_, err := svc.SendRoomMessage(context.Background(), uuid.New(), uuid.New(), "hello")
	if !errors.Is(err, ErrRoomMembershipRequired) {
		t.Fatalf("expected ErrRoomMembershipRequired, got %v", err)
	}
}

func TestMessageServiceSendRoomMessageMembershipCheckFailure(t *testing.T) {
	t.Parallel()

	rr := &fakeMessageRoomRepo{isMemberErr: errors.New("db timeout")}
	svc := NewMessageService(rr, &fakeMessageRepo{}, &fakePublisher{})

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
	svc := NewMessageService(rr, &fakeMessageRepo{}, pub)

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
	svc := NewMessageService(&fakeMessageRoomRepo{}, mr, pub)

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
	svc := NewMessageService(&fakeMessageRoomRepo{}, &fakeMessageRepo{}, &fakePublisher{})

	_, err := svc.SendDM(context.Background(), uid, uid, "ping")
	if !errors.Is(err, ErrDMToSelfForbidden) {
		t.Fatalf("expected ErrDMToSelfForbidden, got %v", err)
	}
}

func TestMessageServiceSendDMPersistenceError(t *testing.T) {
	t.Parallel()

	mr := &fakeMessageRepo{saveErr: errors.New("write failed")}
	svc := NewMessageService(&fakeMessageRoomRepo{}, mr, &fakePublisher{})

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
	svc := NewMessageService(rr, mr, &fakePublisher{})

	roomMsgs, err := svc.RoomHistory(context.Background(), uuid.New(), now, 20)
	if err != nil || len(roomMsgs) != 1 {
		t.Fatalf("room history delegation failed: err=%v len=%d", err, len(roomMsgs))
	}
	dmMsgs, err := svc.DMHistory(context.Background(), uuid.New(), uuid.New(), now, 20)
	if err != nil || len(dmMsgs) != 1 {
		t.Fatalf("dm history delegation failed: err=%v len=%d", err, len(dmMsgs))
	}
}
