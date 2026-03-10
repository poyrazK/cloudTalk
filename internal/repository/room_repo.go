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
	if err != nil {
		return fmt.Errorf("create room: %w", err)
	}
	return nil
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
		return nil, fmt.Errorf("list rooms by user: %w", err)
	}
	defer rows.Close()

	var rooms []*model.Room
	for rows.Next() {
		room := &model.Room{}
		if err := rows.Scan(&room.ID, &room.Name, &room.Description, &room.CreatedBy, &room.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan room row: %w", err)
		}
		rooms = append(rooms, room)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate room rows: %w", err)
	}
	return rooms, nil
}

func (r *RoomRepo) AddMember(ctx context.Context, roomID, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO room_members (room_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		roomID, userID,
	)
	if err != nil {
		return fmt.Errorf("add room member: %w", err)
	}
	return nil
}

func (r *RoomRepo) RemoveMember(ctx context.Context, roomID, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM room_members WHERE room_id=$1 AND user_id=$2`, roomID, userID,
	)
	if err != nil {
		return fmt.Errorf("remove room member: %w", err)
	}
	return nil
}

func (r *RoomRepo) IsMember(ctx context.Context, roomID, userID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM room_members WHERE room_id=$1 AND user_id=$2)`,
		roomID, userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check room membership: %w", err)
	}
	return exists, nil
}

func (r *RoomRepo) ListMessages(ctx context.Context, roomID uuid.UUID, before time.Time, limit int) ([]*model.Message, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, room_id, sender_id, content, created_at, edited_at, deleted_at FROM messages
		 WHERE room_id=$1 AND created_at < $2
		 ORDER BY created_at DESC LIMIT $3`,
		roomID, before, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list room messages: %w", err)
	}
	defer rows.Close()

	var msgs []*model.Message
	for rows.Next() {
		m := &model.Message{}
		if err := rows.Scan(&m.ID, &m.RoomID, &m.SenderID, &m.Content, &m.CreatedAt, &m.EditedAt, &m.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan room message row: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate room message rows: %w", err)
	}
	return msgs, nil
}

func (r *RoomRepo) GetMessageByID(ctx context.Context, id uuid.UUID) (*model.Message, error) {
	m := &model.Message{}
	err := r.db.QueryRow(ctx,
		`SELECT id, room_id, sender_id, content, created_at, edited_at, deleted_at
		 FROM messages WHERE id=$1`,
		id,
	).Scan(&m.ID, &m.RoomID, &m.SenderID, &m.Content, &m.CreatedAt, &m.EditedAt, &m.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("get room message by id: %w", err)
	}
	return m, nil
}

func (r *RoomRepo) UpdateMessageContent(ctx context.Context, id uuid.UUID, content string, editedAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE messages
		 SET content=$2, edited_at=$3
		 WHERE id=$1`,
		id, content, editedAt,
	)
	if err != nil {
		return fmt.Errorf("update room message content: %w", err)
	}
	return nil
}

func (r *RoomRepo) SoftDeleteMessage(ctx context.Context, id uuid.UUID, deletedAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE messages
		 SET deleted_at = COALESCE(deleted_at, $2)
		 WHERE id=$1`,
		id, deletedAt,
	)
	if err != nil {
		return fmt.Errorf("soft delete room message: %w", err)
	}
	return nil
}

func (r *RoomRepo) SaveMessage(ctx context.Context, m *model.Message) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO messages (id, room_id, sender_id, content) VALUES ($1,$2,$3,$4)`,
		m.ID, m.RoomID, m.SenderID, m.Content,
	)
	if err != nil {
		return fmt.Errorf("save room message: %w", err)
	}
	return nil
}
