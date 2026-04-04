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
	createErr            error
	getErr               error
	addMemberErr         error
	addMemberWithRoleErr error
	initReadErr          error
	markReadErr          error
	removeErr            error
	isMember             bool
	isMemberErr          error
	createdRoom          *model.Room
	addedRoomID          uuid.UUID
	addedUserID          uuid.UUID
	initReadRoomID       uuid.UUID
	initReadUserID       uuid.UUID
	unreadResp           []*model.RoomUnreadCount
	conversationResp     []*model.RoomConversationHead
	memberIDsByRoom      map[uuid.UUID][]uuid.UUID
	listMembersResp      []*model.RoomMemberDetail
	rolesByUser          map[uuid.UUID]string
	removedRoomID        uuid.UUID
	removedUserID        uuid.UUID
	listByUserResp       []*model.Room
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

func (f *fakeRoomRepo) AddMemberWithRole(_ context.Context, roomID, userID uuid.UUID, role string) error {
	if f.addMemberWithRoleErr != nil {
		return f.addMemberWithRoleErr
	}
	f.addedRoomID = roomID
	f.addedUserID = userID
	if f.rolesByUser == nil {
		f.rolesByUser = map[uuid.UUID]string{}
	}
	f.rolesByUser[userID] = role
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

func (f *fakeRoomRepo) ListRoomConversationHeads(_ context.Context, _ uuid.UUID, _ int) ([]*model.RoomConversationHead, error) {
	return f.conversationResp, nil
}

func (f *fakeRoomRepo) ListRoomMemberIDs(_ context.Context, roomIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	out := make(map[uuid.UUID][]uuid.UUID, len(roomIDs))
	for _, roomID := range roomIDs {
		out[roomID] = append(out[roomID], f.memberIDsByRoom[roomID]...)
	}
	return out, nil
}

func (f *fakeRoomRepo) ListRoomMembers(_ context.Context, _ uuid.UUID) ([]*model.RoomMemberDetail, error) {
	return f.listMembersResp, nil
}

func (f *fakeRoomRepo) GetMemberRole(_ context.Context, _, userID uuid.UUID) (string, error) {
	role, ok := f.rolesByUser[userID]
	if !ok {
		return "", errors.New("missing role")
	}
	return role, nil
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

type fakeRoomPresenceReader struct {
	online map[uuid.UUID]bool
}

func (f *fakeRoomPresenceReader) IsOnline(userID uuid.UUID) bool {
	return f.online[userID]
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
	if repo.rolesByUser[uid] != model.RoomRoleOwner {
		t.Fatalf("expected creator role owner, got %q", repo.rolesByUser[uid])
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

func TestRoomServiceConversationsComposeUnreadAndLastMessage(t *testing.T) {
	t.Parallel()

	roomAID := uuid.New()
	roomBID := uuid.New()
	repo := &fakeRoomRepo{
		conversationResp: []*model.RoomConversationHead{
			{RoomID: roomAID, Name: "a", Description: "A room", LastMessage: &model.Message{ID: uuid.New(), RoomID: roomAID, Content: "latest-a"}},
			{RoomID: roomBID, Name: "b", Description: "B room", LastMessage: nil},
		},
		unreadResp: []*model.RoomUnreadCount{{RoomID: roomAID, Count: 4}},
		memberIDsByRoom: map[uuid.UUID][]uuid.UUID{
			roomAID: {uuid.New(), uuid.New()},
			roomBID: {uuid.New()},
		},
	}
	pr := &fakeRoomPresenceReader{online: map[uuid.UUID]bool{repo.memberIDsByRoom[roomAID][0]: true}}
	svc := NewRoomServiceWithPresence(repo, pr)

	conversations, err := svc.Conversations(context.Background(), uuid.New(), 50)
	if err != nil {
		t.Fatalf("room conversations failed: %v", err)
	}
	if len(conversations) != 2 {
		t.Fatalf("expected 2 room conversations, got %d", len(conversations))
	}

	if conversations[0].RoomID != roomAID || conversations[0].MemberCount != 2 || conversations[0].UnreadCount != 4 || conversations[0].OnlineCount != 1 || conversations[0].LastMessage == nil || conversations[0].LastMessage.Content != "latest-a" {
		t.Fatalf("unexpected first conversation: %+v", conversations[0])
	}
	if conversations[1].RoomID != roomBID || conversations[1].MemberCount != 1 || conversations[1].UnreadCount != 0 || conversations[1].OnlineCount != 0 || conversations[1].LastMessage != nil {
		t.Fatalf("unexpected second conversation: %+v", conversations[1])
	}
}

func TestRoomServiceMembersComposeOnlineSnapshot(t *testing.T) {
	t.Parallel()

	memberA := uuid.New()
	memberB := uuid.New()
	offlineSeen := time.Now().UTC().Add(-5 * time.Minute)
	repo := &fakeRoomRepo{listMembersResp: []*model.RoomMemberDetail{
		{UserID: memberA, Username: "a", JoinedAt: time.Now().UTC().Add(-2 * time.Minute), LastSeen: &offlineSeen},
		{UserID: memberB, Username: "b", JoinedAt: time.Now().UTC().Add(-1 * time.Minute), LastSeen: &offlineSeen},
	}}
	pr := &fakeRoomPresenceReader{online: map[uuid.UUID]bool{memberA: true}}
	svc := NewRoomServiceWithPresence(repo, pr)

	members, err := svc.Members(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("room members failed: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	if members[0].UserID != memberA || !members[0].Online || members[0].LastSeen != nil {
		t.Fatalf("unexpected first member projection: %+v", members[0])
	}
	if members[1].UserID != memberB || members[1].Online || members[1].LastSeen == nil {
		t.Fatalf("unexpected second member projection: %+v", members[1])
	}
}

func TestRoomServiceRemoveMemberAsOwner(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	memberID := uuid.New()
	repo := &fakeRoomRepo{isMember: true, rolesByUser: map[uuid.UUID]string{ownerID: model.RoomRoleOwner, memberID: model.RoomRoleMember}}
	svc := NewRoomService(repo)
	roomID := uuid.New()

	if err := svc.RemoveMemberAsOwner(context.Background(), roomID, ownerID, memberID); err != nil {
		t.Fatalf("remove member failed: %v", err)
	}
	if repo.removedRoomID != roomID || repo.removedUserID != memberID {
		t.Fatalf("unexpected remove delegation: room=%s user=%s", repo.removedRoomID, repo.removedUserID)
	}
}

func TestRoomServiceRemoveMemberAsOwnerRejectsNonOwnerAndOwnerRemoval(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	memberID := uuid.New()
	repo := &fakeRoomRepo{isMember: true, rolesByUser: map[uuid.UUID]string{ownerID: model.RoomRoleOwner, memberID: model.RoomRoleMember}}
	svc := NewRoomService(repo)
	roomID := uuid.New()

	if err := svc.RemoveMemberAsOwner(context.Background(), roomID, memberID, ownerID); err == nil {
		t.Fatal("expected non-owner moderation failure")
	}
	if err := svc.RemoveMemberAsOwner(context.Background(), roomID, ownerID, ownerID); err == nil {
		t.Fatal("expected self-remove moderation failure")
	}

	outsiderID := uuid.New()
	if err := svc.RemoveMemberAsOwner(context.Background(), roomID, outsiderID, memberID); err == nil {
		t.Fatal("expected outsider moderation failure")
	}
}
