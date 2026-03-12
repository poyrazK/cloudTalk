package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poyrazk/cloudtalk/internal/model"
)

type UserRepo struct{ db *pgxpool.Pool }

func NewUserRepo(db *pgxpool.Pool) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) Create(ctx context.Context, u *model.User) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO users (id, username, email, password_hash) VALUES ($1,$2,$3,$4)`,
		u.ID, u.Username, u.Email, u.PasswordHash,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(ctx,
		`SELECT id, username, email, password_hash, last_seen_at, created_at FROM users WHERE id=$1`, id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.LastSeenAt, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(ctx,
		`SELECT id, username, email, password_hash, last_seen_at, created_at FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.LastSeenAt, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

// --- Refresh tokens ---

func (r *UserRepo) SaveRefreshToken(ctx context.Context, t *model.RefreshToken) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at) VALUES ($1,$2,$3,$4)`,
		t.ID, t.UserID, t.TokenHash, t.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

func (r *UserRepo) GetRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	t := &model.RefreshToken{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at FROM refresh_tokens WHERE token_hash=$1`, tokenHash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("get refresh token: %w", err)
	}
	return t, nil
}

func (r *UserRepo) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash=$1`, tokenHash)
	if err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}
	return nil
}

func (r *UserRepo) DeleteExpiredRefreshTokens(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `DELETE FROM refresh_tokens WHERE expires_at < $1`, time.Now())
	if err != nil {
		return fmt.Errorf("delete expired refresh tokens: %w", err)
	}
	return nil
}

func (r *UserRepo) UpdateLastSeen(ctx context.Context, userID uuid.UUID, at time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET last_seen_at = $2 WHERE id = $1`, userID, at)
	if err != nil {
		return fmt.Errorf("update user last seen: %w", err)
	}
	return nil
}
