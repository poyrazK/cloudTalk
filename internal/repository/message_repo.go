package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poyrazk/cloudtalk/internal/model"
)

type MessageRepo struct{ db *pgxpool.Pool }

func NewMessageRepo(db *pgxpool.Pool) *MessageRepo { return &MessageRepo{db: db} }

func (r *MessageRepo) SaveDM(ctx context.Context, m *model.DirectMessage) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO direct_messages (id, sender_id, receiver_id, content) VALUES ($1,$2,$3,$4)`,
		m.ID, m.SenderID, m.ReceiverID, m.Content,
	)
	if err != nil {
		return fmt.Errorf("save dm: %w", err)
	}
	return nil
}

// ListDMs returns paginated DMs between two users, ordered newest-first.
func (r *MessageRepo) ListDMs(ctx context.Context, userA, userB uuid.UUID, before time.Time, limit int) ([]*model.DirectMessage, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, sender_id, receiver_id, content, created_at
		 FROM direct_messages
		 WHERE ((sender_id=$1 AND receiver_id=$2) OR (sender_id=$2 AND receiver_id=$1))
		   AND created_at < $3
		 ORDER BY created_at DESC LIMIT $4`,
		userA, userB, before, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query dms: %w", err)
	}
	defer rows.Close()

	var msgs []*model.DirectMessage
	for rows.Next() {
		m := &model.DirectMessage{}
		if err := rows.Scan(&m.ID, &m.SenderID, &m.ReceiverID, &m.Content, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan dm row: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dm rows: %w", err)
	}
	return msgs, nil
}
