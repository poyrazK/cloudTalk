package model

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `db:"id"            json:"id"`
	Username     string    `db:"username"      json:"username"`
	Email        string    `db:"email"         json:"email"`
	PasswordHash string    `db:"password_hash" json:"-"`
	CreatedAt    time.Time `db:"created_at"    json:"created_at"`
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
	JoinedAt time.Time `db:"joined_at" json:"joined_at"`
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

type RefreshToken struct {
	ID        uuid.UUID `db:"id"`
	UserID    uuid.UUID `db:"user_id"`
	TokenHash string    `db:"token_hash"`
	ExpiresAt time.Time `db:"expires_at"`
}
