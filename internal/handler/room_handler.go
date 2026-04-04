package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	authsvc "github.com/poyrazk/cloudtalk/internal/auth"
	"github.com/poyrazk/cloudtalk/internal/service"
)

type RoomHandler struct {
	rooms    *service.RoomService
	messages *service.MessageService
}

func NewRoomHandler(rooms *service.RoomService, messages *service.MessageService) *RoomHandler {
	return &RoomHandler{rooms: rooms, messages: messages}
}

func (h *RoomHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, _ := authsvc.UserIDFromContext(r.Context())
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := validateRoom(req.Name, req.Description); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	room, err := h.rooms.Create(r.Context(), req.Name, req.Description, userID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusCreated, room)
}

func (h *RoomHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := authsvc.UserIDFromContext(r.Context())
	rooms, err := h.rooms.ListByUser(r.Context(), userID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusOK, rooms)
}

func (h *RoomHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid room id", http.StatusBadRequest)
		return
	}
	room, err := h.rooms.Get(r.Context(), id)
	if err != nil {
		jsonError(w, "room not found", http.StatusNotFound)
		return
	}
	jsonResp(w, http.StatusOK, room)
}

func (h *RoomHandler) Join(w http.ResponseWriter, r *http.Request) {
	userID, _ := authsvc.UserIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid room id", http.StatusBadRequest)
		return
	}
	if err := h.rooms.Join(r.Context(), id, userID); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RoomHandler) Leave(w http.ResponseWriter, r *http.Request) {
	userID, _ := authsvc.UserIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid room id", http.StatusBadRequest)
		return
	}
	if err := h.rooms.Leave(r.Context(), id, userID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrRoomBadRequestOwnerCannotLeave) {
			status = http.StatusBadRequest
		}
		jsonError(w, err.Error(), status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RoomHandler) Messages(w http.ResponseWriter, r *http.Request) {
	userID, _ := authsvc.UserIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid room id", http.StatusBadRequest)
		return
	}

	// Only room members may read history.
	ok, err := h.rooms.IsMember(r.Context(), id, userID)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		jsonError(w, "forbidden", http.StatusForbidden)
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
	msgs, err := h.messages.RoomHistory(r.Context(), id, before, limit)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusOK, msgs)
}

func (h *RoomHandler) Members(w http.ResponseWriter, r *http.Request) {
	userID, _ := authsvc.UserIDFromContext(r.Context())
	roomID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid room id", http.StatusBadRequest)
		return
	}

	ok, err := h.rooms.IsMember(r.Context(), roomID, userID)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}

	members, err := h.rooms.Members(r.Context(), roomID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusOK, members)
}

func (h *RoomHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	actorID, _ := authsvc.UserIDFromContext(r.Context())
	roomID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid room id", http.StatusBadRequest)
		return
	}
	targetUserID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		jsonError(w, "invalid user id", http.StatusBadRequest)
		return
	}

	if err := h.rooms.RemoveMemberAsOwner(r.Context(), roomID, actorID, targetUserID); err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, service.ErrRoomModerationForbidden):
			status = http.StatusForbidden
		case errors.Is(err, service.ErrRoomBadRequestUseLeave), errors.Is(err, service.ErrRoomBadRequestOwnerCannotBeRemoved), errors.Is(err, service.ErrRoomBadRequestTargetNotMember):
			status = http.StatusBadRequest
		}
		jsonError(w, err.Error(), status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RoomHandler) UnreadCounts(w http.ResponseWriter, r *http.Request) {
	userID, _ := authsvc.UserIDFromContext(r.Context())
	counts, err := h.rooms.UnreadCounts(r.Context(), userID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusOK, counts)
}

func (h *RoomHandler) Conversations(w http.ResponseWriter, r *http.Request) {
	userID, _ := authsvc.UserIDFromContext(r.Context())
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	conversations, err := h.rooms.Conversations(r.Context(), userID, limit)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusOK, conversations)
}
