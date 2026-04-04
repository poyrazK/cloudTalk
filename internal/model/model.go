package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	RoomRoleOwner  = "owner"
	RoomRoleMember = "member"
)

type User struct {
	ID           uuid.UUID  `db:"id"            json:"id"`
	Username     string     `db:"username"      json:"username"`
	Email        string     `db:"email"         json:"email"`
	PasswordHash string     `db:"password_hash" json:"-"`
	LastSeenAt   *time.Time `db:"last_seen_at"  json:"last_seen,omitempty"`
	CreatedAt    time.Time  `db:"created_at"    json:"created_at"`
}

type Room struct {
	ID          uuid.UUID `db:"id"          json:"id"`
	Name        string    `db:"name"        json:"name"`
	Description string    `db:"description" json:"description"`
	CreatedBy   uuid.UUID `db:"created_by"  json:"created_by"`
	CreatedAt   time.Time `db:"created_at"  json:"created_at"`
}

type RoomMember struct {
	RoomID   uuid.UUID `db:"room_id"  json:"room_id"`
	UserID   uuid.UUID `db:"user_id"  json:"user_id"`
	Role     string    `db:"role"     json:"role"`
	JoinedAt time.Time `db:"joined_at" json:"joined_at"`
}

type RoomMemberDetail struct {
	UserID   uuid.UUID  `db:"user_id"   json:"user_id"`
	Username string     `db:"username"  json:"username"`
	Role     string     `db:"role"      json:"role"`
	JoinedAt time.Time  `db:"joined_at" json:"joined_at"`
	LastSeen *time.Time `db:"last_seen_at" json:"last_seen"`
	Online   bool       `json:"online"`
}

type RoomUnreadCount struct {
	RoomID uuid.UUID `db:"room_id" json:"room_id"`
	Count  int       `db:"count"   json:"count"`
}

type RoomConversationHead struct {
	RoomID      uuid.UUID `db:"room_id"     json:"room_id"`
	Name        string    `db:"name"        json:"name"`
	Description string    `db:"description" json:"description"`
	LastMessage *Message  `json:"last_message"`
}

type RoomConversation struct {
	RoomID      uuid.UUID `json:"room_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	MemberCount int       `json:"member_count"`
	OnlineCount int       `json:"online_count"`
	UnreadCount int       `json:"unread_count"`
	LastMessage *Message  `json:"last_message"`
}

type Message struct {
	ID        uuid.UUID  `db:"id"         json:"id"`
	RoomID    uuid.UUID  `db:"room_id"    json:"room_id"`
	SenderID  uuid.UUID  `db:"sender_id"  json:"sender_id"`
	Content   string     `db:"content"    json:"content"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	EditedAt  *time.Time `db:"edited_at"  json:"edited_at,omitempty"`
	DeletedAt *time.Time `db:"deleted_at" json:"deleted_at,omitempty"`
}

type DirectMessage struct {
	ID          uuid.UUID  `db:"id"           json:"id"`
	SenderID    uuid.UUID  `db:"sender_id"    json:"sender_id"`
	ReceiverID  uuid.UUID  `db:"receiver_id"  json:"receiver_id"`
	Content     string     `db:"content"      json:"content"`
	CreatedAt   time.Time  `db:"created_at"   json:"created_at"`
	DeliveredAt *time.Time `db:"delivered_at" json:"delivered_at,omitempty"`
	ReadAt      *time.Time `db:"read_at"      json:"read_at,omitempty"`
	EditedAt    *time.Time `db:"edited_at"    json:"edited_at,omitempty"`
	DeletedAt   *time.Time `db:"deleted_at"   json:"deleted_at,omitempty"`
}

type DMUnreadCount struct {
	UserID uuid.UUID `db:"user_id" json:"user_id"`
	Count  int       `db:"count"   json:"count"`
}

type DMConversationHead struct {
	UserID      uuid.UUID      `db:"user_id" json:"user_id"`
	LastMessage *DirectMessage `json:"last_message"`
}

type DMConversation struct {
	UserID      uuid.UUID      `json:"user_id"`
	Username    string         `json:"username"`
	Online      bool           `json:"online"`
	LastSeen    *time.Time     `json:"last_seen"`
	UnreadCount int            `json:"unread_count"`
	LastMessage *DirectMessage `json:"last_message"`
}

type RefreshToken struct {
	ID        uuid.UUID `db:"id"`
	UserID    uuid.UUID `db:"user_id"`
	TokenHash string    `db:"token_hash"`
	ExpiresAt time.Time `db:"expires_at"`
}
