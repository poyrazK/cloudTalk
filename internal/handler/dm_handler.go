package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	authsvc "github.com/poyrazk/cloudtalk/internal/auth"
	"github.com/poyrazk/cloudtalk/internal/service"
)

type DMHandler struct {
	messages *service.MessageService
}

func NewDMHandler(messages *service.MessageService) *DMHandler {
	return &DMHandler{messages: messages}
}

func (h *DMHandler) Messages(w http.ResponseWriter, r *http.Request) {
	callerID, _ := authsvc.UserIDFromContext(r.Context())
	otherID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		jsonError(w, "invalid user id", http.StatusBadRequest)
		return
	}

	// The SQL query filters by callerID (from JWT) so only messages where
	// the authenticated user is sender or receiver are returned.
	// Prevent querying DMs with yourself.
	if callerID == otherID {
		jsonError(w, "cannot fetch DMs with yourself", http.StatusBadRequest)
		return
	}

	before := time.Now()
	if b := r.URL.Query().Get("before"); b != "" {
		if t, err := time.Parse(time.RFC3339Nano, b); err == nil {
			before = t
		}
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	msgs, err := h.messages.DMHistory(r.Context(), callerID, otherID, before, limit)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusOK, msgs)
}

func (h *DMHandler) UnreadCounts(w http.ResponseWriter, r *http.Request) {
	callerID, _ := authsvc.UserIDFromContext(r.Context())
	counts, err := h.messages.DMUnreadCounts(r.Context(), callerID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusOK, counts)
}
