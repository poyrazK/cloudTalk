package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poyrazk/cloudtalk/internal/model"
)

type UserRepo struct{ db *pgxpool.Pool }

func NewUserRepo(db *pgxpool.Pool) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) Create(ctx context.Context, u *model.User) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO users (id, username, display_name, avatar_url, email, password_hash, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		u.ID, u.Username, u.DisplayName, u.AvatarURL, u.Email, u.PasswordHash, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	return r.getAndHydrateUser(ctx, `SELECT id, username, display_name, avatar_url, email, password_hash, last_seen_at, created_at, updated_at FROM users WHERE id=$1`, id)
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	return r.getAndHydrateUser(ctx, `SELECT id, username, display_name, avatar_url, email, password_hash, last_seen_at, created_at, updated_at FROM users WHERE email=$1`, email)
}

func (r *UserRepo) getAndHydrateUser(ctx context.Context, query string, args ...any) (*model.User, error) {
	u := &model.User{}
	var avatarURL pgtype.Text
	if err := r.db.QueryRow(ctx, query, args...).Scan(&u.ID, &u.Username, &u.DisplayName, &avatarURL, &u.Email, &u.PasswordHash, &u.LastSeenAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	if avatarURL.Valid {
		u.AvatarURL = &avatarURL.String
	}
	if u.DisplayName == "" {
		u.DisplayName = u.Username
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
	_, err := r.db.Exec(ctx,
		`UPDATE users
		 SET last_seen_at = GREATEST(COALESCE(last_seen_at, $2), $2)
		 WHERE id = $1`,
		userID, at,
	)
	if err != nil {
		return fmt.Errorf("update user last seen: %w", err)
	}
	return nil
}
