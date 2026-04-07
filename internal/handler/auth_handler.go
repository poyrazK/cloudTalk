package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	authsvc "github.com/poyrazk/cloudtalk/internal/auth"
	"github.com/poyrazk/cloudtalk/internal/repository"
)

type AuthHandler struct {
	auth     *authsvc.Service
	userRepo *repository.UserRepo
}

func NewAuthHandler(auth *authsvc.Service, userRepo *repository.UserRepo) *AuthHandler {
	return &AuthHandler{auth: auth, userRepo: userRepo}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := validateRegister(req.Username, req.Email, req.Password); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	u, err := h.auth.Register(r.Context(), req.Username, strings.TrimSpace(req.Email), req.Password)
	if err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	jsonResp(w, http.StatusCreated, authUserResponseFromModel(u))
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	access, refresh, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	jsonResp(w, http.StatusOK, map[string]string{
		"access_token":  access,
		"refresh_token": refresh,
	})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	access, err := h.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		jsonError(w, "invalid or expired refresh token", http.StatusUnauthorized)
		return
	}
	jsonResp(w, http.StatusOK, map[string]string{"access_token": access})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	_ = h.auth.Logout(r.Context(), req.RefreshToken)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, _ := authsvc.UserIDFromContext(r.Context())
	u, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonError(w, "user not found", http.StatusNotFound)
			return
		}
		slog.Error("me: get user", "err", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusOK, authUserResponseFromModel(u))
}

func (h *AuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := authsvc.UserIDFromContext(r.Context())
	var req UpdateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := h.userRepo.UpdateProfileFields(r.Context(), userID, req.DisplayName, req.AvatarURL); err != nil {
		slog.Error("update me: update profile", "err", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	u, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonError(w, "user not found", http.StatusNotFound)
			return
		}
		slog.Error("update me: get user", "err", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusOK, authUserResponseFromModel(u))
}

func (h *AuthHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid user id", http.StatusBadRequest)
		return
	}
	u, err := h.userRepo.GetPublicProfileByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonError(w, "user not found", http.StatusNotFound)
			return
		}
		slog.Error("get user: lookup", "err", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonResp(w, http.StatusOK, publicProfileResponseFromModel(u))
}

// --- helpers ---

func jsonResp(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	jsonResp(w, status, map[string]string{"error": msg})
}
