package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	authsvc "github.com/poyrazk/cloudtalk/internal/auth"
)

func TestAuthenticatedRateLimitUsesUserScope(t *testing.T) {
	t.Parallel()

	mw := AuthenticatedRateLimit("conversation_reads", 1)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	userID := uuid.New()
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/dms/conversations", nil).WithContext(context.WithValue(context.Background(), authsvc.UserIDKey, userID))
	req1.RemoteAddr = "127.0.0.1:1234"
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected first request ok, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/dms/conversations", nil).WithContext(context.WithValue(context.Background(), authsvc.UserIDKey, userID))
	req2.RemoteAddr = "127.0.0.2:1234"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second same-user request throttled, got %d", rec2.Code)
	}

	otherReq := httptest.NewRequest(http.MethodGet, "/api/v1/dms/conversations", nil).WithContext(context.WithValue(context.Background(), authsvc.UserIDKey, uuid.New()))
	otherReq.RemoteAddr = "127.0.0.3:1234"
	otherRec := httptest.NewRecorder()
	h.ServeHTTP(otherRec, otherReq)
	if otherRec.Code != http.StatusOK {
		t.Fatalf("expected other user request ok, got %d", otherRec.Code)
	}
}

func TestAuthenticatedRateLimitFallsBackToIP(t *testing.T) {
	t.Parallel()

	mw := AuthenticatedRateLimit("message_history_reads", 1)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/1/messages", nil)
	req1.RemoteAddr = "127.0.0.1:1234"
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected first request ok, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/1/messages", nil)
	req2.RemoteAddr = "127.0.0.1:4567"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second same-ip request throttled, got %d", rec2.Code)
	}

	var payload map[string]string
	if err := json.Unmarshal(rec2.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal throttle response: %v", err)
	}
	if payload["error"] != "too many requests" {
		t.Fatalf("unexpected throttle error payload: %+v", payload)
	}
}
