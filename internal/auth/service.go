package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/model"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrTokenExpired = errors.New("token expired")

type Service struct {
	users          userStore
	jwtSecret      []byte
	accessExpMin   int
	refreshExpDays int
}

type userStore interface {
	Create(ctx context.Context, u *model.User) error
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	SaveRefreshToken(ctx context.Context, t *model.RefreshToken) error
	GetRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, tokenHash string) error
}

func NewService(users userStore, secret string, accessExpMin, refreshExpDays int) *Service {
	return &Service{
		users:          users,
		jwtSecret:      []byte(secret),
		accessExpMin:   accessExpMin,
		refreshExpDays: refreshExpDays,
	}
}

// Register creates a new user and returns it.
func (s *Service) Register(ctx context.Context, username, email, password string) (*model.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	u := &model.User{
		ID:           uuid.New(),
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	return u, nil
}

// Login validates credentials and returns (accessToken, refreshToken, error).
func (s *Service) Login(ctx context.Context, email, password string) (string, string, error) {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return "", "", ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", "", ErrInvalidCredentials
	}

	access, err := s.generateAccessToken(u.ID)
	if err != nil {
		return "", "", err
	}

	refresh, err := s.issueRefreshToken(ctx, u.ID)
	if err != nil {
		return "", "", err
	}

	return access, refresh, nil
}

// Refresh validates a refresh token and issues a new access token.
func (s *Service) Refresh(ctx context.Context, rawRefresh string) (string, error) {
	hash := hashToken(rawRefresh)
	rt, err := s.users.GetRefreshToken(ctx, hash)
	if err != nil {
		return "", ErrInvalidCredentials
	}
	if time.Now().After(rt.ExpiresAt) {
		_ = s.users.DeleteRefreshToken(ctx, hash)
		return "", ErrTokenExpired
	}
	return s.generateAccessToken(rt.UserID)
}

// Logout invalidates the given refresh token.
func (s *Service) Logout(ctx context.Context, rawRefresh string) error {
	if err := s.users.DeleteRefreshToken(ctx, hashToken(rawRefresh)); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}

// ValidateAccessToken parses and validates a JWT access token, returning the user ID.
func (s *Service) ValidateAccessToken(tokenStr string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse access token: %w", err)
	}
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return uuid.Nil, errors.New("invalid token")
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse token subject: %w", err)
	}
	return id, nil
}

func (s *Service) generateAccessToken(userID uuid.UUID) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.accessExpMin) * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return tok, nil
}

func (s *Service) issueRefreshToken(ctx context.Context, userID uuid.UUID) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate refresh token bytes: %w", err)
	}
	rawStr := hex.EncodeToString(raw)
	rt := &model.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: hashToken(rawStr),
		ExpiresAt: time.Now().AddDate(0, 0, s.refreshExpDays),
	}
	if err := s.users.SaveRefreshToken(ctx, rt); err != nil {
		return "", fmt.Errorf("save refresh token: %w", err)
	}
	return rawStr, nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
