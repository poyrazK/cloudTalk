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
	ID        uuid.UUID `db:"id"         json:"id"`
	RoomID    uuid.UUID `db:"room_id"    json:"room_id"`
	SenderID  uuid.UUID `db:"sender_id"  json:"sender_id"`
	Content   string    `db:"content"    json:"content"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type DirectMessage struct {
	ID         uuid.UUID `db:"id"          json:"id"`
	SenderID   uuid.UUID `db:"sender_id"   json:"sender_id"`
	ReceiverID uuid.UUID `db:"receiver_id" json:"receiver_id"`
	Content    string    `db:"content"     json:"content"`
	CreatedAt  time.Time `db:"created_at"  json:"created_at"`
}

type RefreshToken struct {
	ID        uuid.UUID `db:"id"`
	UserID    uuid.UUID `db:"user_id"`
	TokenHash string    `db:"token_hash"`
	ExpiresAt time.Time `db:"expires_at"`
}
