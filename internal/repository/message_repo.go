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

func (r *MessageRepo) GetDMByID(ctx context.Context, id uuid.UUID) (*model.DirectMessage, error) {
	dm := &model.DirectMessage{}
	err := r.db.QueryRow(ctx,
		`SELECT id, sender_id, receiver_id, content, created_at, delivered_at, read_at, edited_at, deleted_at
		 FROM direct_messages
		 WHERE id=$1`,
		id,
	).Scan(&dm.ID, &dm.SenderID, &dm.ReceiverID, &dm.Content, &dm.CreatedAt, &dm.DeliveredAt, &dm.ReadAt, &dm.EditedAt, &dm.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("get dm by id: %w", err)
	}
	return dm, nil
}

func (r *MessageRepo) MarkDMDelivered(ctx context.Context, dmID, receiverID uuid.UUID, at time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE direct_messages
		 SET delivered_at = COALESCE(delivered_at, $3)
		 WHERE id=$1 AND receiver_id=$2`,
		dmID, receiverID, at,
	)
	if err != nil {
		return fmt.Errorf("mark dm delivered: %w", err)
	}
	return nil
}

func (r *MessageRepo) MarkDMRead(ctx context.Context, dmID, receiverID uuid.UUID, at time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE direct_messages
		 SET delivered_at = COALESCE(delivered_at, $3),
		     read_at = COALESCE(read_at, $3)
		 WHERE id=$1 AND receiver_id=$2`,
		dmID, receiverID, at,
	)
	if err != nil {
		return fmt.Errorf("mark dm read: %w", err)
	}
	return nil
}

func (r *MessageRepo) ListDMUnreadCounts(ctx context.Context, userID uuid.UUID) ([]*model.DMUnreadCount, error) {
	rows, err := r.db.Query(ctx,
		`SELECT sender_id AS user_id, COUNT(*)::int AS count
		 FROM direct_messages
		 WHERE receiver_id=$1 AND read_at IS NULL
		 GROUP BY sender_id
		 ORDER BY count DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list dm unread counts: %w", err)
	}
	defer rows.Close()

	var counts []*model.DMUnreadCount
	for rows.Next() {
		c := &model.DMUnreadCount{}
		if err := rows.Scan(&c.UserID, &c.Count); err != nil {
			return nil, fmt.Errorf("scan dm unread count row: %w", err)
		}
		counts = append(counts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dm unread count rows: %w", err)
	}
	return counts, nil
}

func (r *MessageRepo) UpdateDMContent(ctx context.Context, id uuid.UUID, content string, editedAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE direct_messages
		 SET content=$2, edited_at=$3
		 WHERE id=$1`,
		id, content, editedAt,
	)
	if err != nil {
		return fmt.Errorf("update dm content: %w", err)
	}
	return nil
}

func (r *MessageRepo) SoftDeleteDM(ctx context.Context, id uuid.UUID, deletedAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE direct_messages
		 SET deleted_at = COALESCE(deleted_at, $2)
		 WHERE id=$1`,
		id, deletedAt,
	)
	if err != nil {
		return fmt.Errorf("soft delete dm: %w", err)
	}
	return nil
}

// ListDMs returns paginated DMs between two users, ordered newest-first.
func (r *MessageRepo) ListDMs(ctx context.Context, userA, userB uuid.UUID, before time.Time, limit int) ([]*model.DirectMessage, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, sender_id, receiver_id, content, created_at, delivered_at, read_at, edited_at, deleted_at
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
		if err := rows.Scan(&m.ID, &m.SenderID, &m.ReceiverID, &m.Content, &m.CreatedAt, &m.DeliveredAt, &m.ReadAt, &m.EditedAt, &m.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan dm row: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dm rows: %w", err)
	}
	return msgs, nil
}
