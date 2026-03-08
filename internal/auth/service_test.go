package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/poyrazk/cloudtalk/internal/model"
	"golang.org/x/crypto/bcrypt"
)

type fakeUserStore struct {
	usersByEmail     map[string]*model.User
	refreshByHash    map[string]*model.RefreshToken
	createdUser      *model.User
	deletedTokenHash string
	createErr        error
	getByEmailErr    error
	saveRefreshErr   error
	getRefreshErr    error
	deleteRefreshErr error
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{
		usersByEmail:  make(map[string]*model.User),
		refreshByHash: make(map[string]*model.RefreshToken),
	}
}

func (f *fakeUserStore) Create(_ context.Context, u *model.User) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createdUser = u
	f.usersByEmail[u.Email] = u
	return nil
}

func (f *fakeUserStore) GetByEmail(_ context.Context, email string) (*model.User, error) {
	if f.getByEmailErr != nil {
		return nil, f.getByEmailErr
	}
	u, ok := f.usersByEmail[email]
	if !ok {
		return nil, errors.New("not found")
	}
	return u, nil
}

func (f *fakeUserStore) SaveRefreshToken(_ context.Context, t *model.RefreshToken) error {
	if f.saveRefreshErr != nil {
		return f.saveRefreshErr
	}
	f.refreshByHash[t.TokenHash] = t
	return nil
}

func (f *fakeUserStore) GetRefreshToken(_ context.Context, tokenHash string) (*model.RefreshToken, error) {
	if f.getRefreshErr != nil {
		return nil, f.getRefreshErr
	}
	rt, ok := f.refreshByHash[tokenHash]
	if !ok {
		return nil, errors.New("not found")
	}
	return rt, nil
}

func (f *fakeUserStore) DeleteRefreshToken(_ context.Context, tokenHash string) error {
	if f.deleteRefreshErr != nil {
		return f.deleteRefreshErr
	}
	f.deletedTokenHash = tokenHash
	delete(f.refreshByHash, tokenHash)
	return nil
}

func TestServiceRegisterHashesPassword(t *testing.T) {
	store := newFakeUserStore()
	svc := NewService(store, "secret", 15, 7)

	u, err := svc.Register(context.Background(), "alice", "alice@example.com", "super-secret")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if u.ID == uuid.Nil {
		t.Fatal("expected generated user ID")
	}
	if u.PasswordHash == "super-secret" {
		t.Fatal("password should be hashed")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("super-secret")); err != nil {
		t.Fatalf("password hash does not match: %v", err)
	}
}

func TestServiceLoginSuccessAndRefreshStored(t *testing.T) {
	store := newFakeUserStore()
	hash, err := bcrypt.GenerateFromPassword([]byte("pass12345"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	uid := uuid.New()
	store.usersByEmail["bob@example.com"] = &model.User{ID: uid, Email: "bob@example.com", PasswordHash: string(hash)}

	svc := NewService(store, "secret", 15, 7)
	access, refresh, err := svc.Login(context.Background(), "bob@example.com", "pass12345")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if access == "" || refresh == "" {
		t.Fatal("expected non-empty access and refresh tokens")
	}
	if _, err := svc.ValidateAccessToken(access); err != nil {
		t.Fatalf("access token should validate: %v", err)
	}
	if _, ok := store.refreshByHash[hashToken(refresh)]; !ok {
		t.Fatal("expected refresh token hash to be persisted")
	}
}

func TestServiceLoginInvalidCredentials(t *testing.T) {
	store := newFakeUserStore()
	svc := NewService(store, "secret", 15, 7)

	_, _, err := svc.Login(context.Background(), "none@example.com", "bad")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestServiceRefreshExpiredToken(t *testing.T) {
	store := newFakeUserStore()
	svc := NewService(store, "secret", 15, 7)
	uid := uuid.New()
	raw := "raw-refresh"
	h := hashToken(raw)
	store.refreshByHash[h] = &model.RefreshToken{UserID: uid, TokenHash: h, ExpiresAt: time.Now().Add(-1 * time.Minute)}

	_, err := svc.Refresh(context.Background(), raw)
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
	if store.deletedTokenHash != h {
		t.Fatalf("expected token hash %s to be deleted", h)
	}
}

func TestServiceRefreshSuccess(t *testing.T) {
	store := newFakeUserStore()
	svc := NewService(store, "secret", 15, 7)
	uid := uuid.New()
	raw := "raw-refresh-ok"
	h := hashToken(raw)
	store.refreshByHash[h] = &model.RefreshToken{UserID: uid, TokenHash: h, ExpiresAt: time.Now().Add(30 * time.Minute)}

	access, err := svc.Refresh(context.Background(), raw)
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	gotID, err := svc.ValidateAccessToken(access)
	if err != nil {
		t.Fatalf("validate access: %v", err)
	}
	if gotID != uid {
		t.Fatalf("expected user %s, got %s", uid, gotID)
	}
}

func TestServiceValidateAccessTokenInvalid(t *testing.T) {
	svc := NewService(newFakeUserStore(), "secret", 15, 7)
	if _, err := svc.ValidateAccessToken("not-a-jwt"); err == nil {
		t.Fatal("expected parse error")
	}
}
