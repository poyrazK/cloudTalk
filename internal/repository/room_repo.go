package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poyrazk/cloudtalk/internal/model"
)

type RoomRepo struct{ db *pgxpool.Pool }

func NewRoomRepo(db *pgxpool.Pool) *RoomRepo { return &RoomRepo{db: db} }

func (r *RoomRepo) Create(ctx context.Context, room *model.Room) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO rooms (id, name, description, created_by) VALUES ($1,$2,$3,$4)`,
		room.ID, room.Name, room.Description, room.CreatedBy,
	)
	return err
}

func (r *RoomRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Room, error) {
	room := &model.Room{}
	err := r.db.QueryRow(ctx,
		`SELECT id, name, description, created_by, created_at FROM rooms WHERE id=$1`, id,
	).Scan(&room.ID, &room.Name, &room.Description, &room.CreatedBy, &room.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("room not found: %w", err)
	}
	return room, nil
}

func (r *RoomRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]*model.Room, error) {
	rows, err := r.db.Query(ctx,
		`SELECT r.id, r.name, r.description, r.created_by, r.created_at
		 FROM rooms r JOIN room_members m ON r.id=m.room_id
		 WHERE m.user_id=$1 ORDER BY r.created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []*model.Room
	for rows.Next() {
		room := &model.Room{}
		if err := rows.Scan(&room.ID, &room.Name, &room.Description, &room.CreatedBy, &room.CreatedAt); err != nil {
			return nil, err
		}
		rooms = append(rooms, room)
	}
	return rooms, rows.Err()
}

func (r *RoomRepo) AddMember(ctx context.Context, roomID, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO room_members (room_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		roomID, userID,
	)
	return err
}

func (r *RoomRepo) RemoveMember(ctx context.Context, roomID, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM room_members WHERE room_id=$1 AND user_id=$2`, roomID, userID,
	)
	return err
}

func (r *RoomRepo) IsMember(ctx context.Context, roomID, userID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM room_members WHERE room_id=$1 AND user_id=$2)`,
		roomID, userID,
	).Scan(&exists)
	return exists, err
}

func (r *RoomRepo) ListMessages(ctx context.Context, roomID uuid.UUID, before time.Time, limit int) ([]*model.Message, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, room_id, sender_id, content, created_at FROM messages
		 WHERE room_id=$1 AND created_at < $2
		 ORDER BY created_at DESC LIMIT $3`,
		roomID, before, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*model.Message
	for rows.Next() {
		m := &model.Message{}
		if err := rows.Scan(&m.ID, &m.RoomID, &m.SenderID, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (r *RoomRepo) SaveMessage(ctx context.Context, m *model.Message) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO messages (id, room_id, sender_id, content) VALUES ($1,$2,$3,$4)`,
		m.ID, m.RoomID, m.SenderID, m.Content,
	)
	return err
}
