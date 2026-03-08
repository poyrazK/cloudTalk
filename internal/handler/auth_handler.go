package handler

import (
	"encoding/json"
	"net/http"

	authsvc "github.com/poyrazk/cloudtalk/internal/auth"
)

type AuthHandler struct {
	auth *authsvc.Service
}

func NewAuthHandler(auth *authsvc.Service) *AuthHandler {
	return &AuthHandler{auth: auth}
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
	u, err := h.auth.Register(r.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	jsonResp(w, http.StatusCreated, u)
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

// --- helpers ---

func jsonResp(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	jsonResp(w, status, map[string]string{"error": msg})
}
