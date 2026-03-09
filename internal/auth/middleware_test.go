package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestMiddlewareUnauthorizedWithoutBearer(t *testing.T) {
	svc := NewService(newFakeUserStore(), "secret", 15, 7)
	mw := Middleware(svc)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMiddlewareInjectsUserID(t *testing.T) {
	svc := NewService(newFakeUserStore(), "secret", 15, 7)
	uid := uuid.New()
	tok, err := svc.generateAccessToken(uid)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	mw := Middleware(svc)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID, ok := UserIDFromContext(r.Context())
		if !ok {
			t.Fatal("expected user id in context")
		}
		if ctxID != uid {
			t.Fatalf("expected user %s, got %s", uid, ctxID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}
