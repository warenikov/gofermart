// Package auth содержит HTTP-хендлеры регистрации и логина.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/domain"
	"github.com/warenikov/gofermart/internal/logger"
	authsvc "github.com/warenikov/gofermart/internal/service/auth"
)

// Service — поведение, нужное хендлерам auth (для тестируемости).
type Service interface {
	Register(ctx context.Context, login, password string) (string, error)
	Login(ctx context.Context, login, password string) (string, error)
}

type credentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
}

// Handler — HTTP-обвязка над auth-сервисом.
type Handler struct {
	svc Service
	log *zap.Logger
}

// NewHandler создаёт HTTP-хендлер.
func NewHandler(svc Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: logger.For(log, "handler.auth")}
}

// Register обрабатывает POST /api/user/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	creds, ok := decodeCreds(w, r)
	if !ok {
		return
	}
	token, err := h.svc.Register(r.Context(), creds.Login, creds.Password)
	if err != nil {
		h.handleRegisterError(w, err)
		return
	}
	writeToken(w, token)
}

// Login обрабатывает POST /api/user/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	creds, ok := decodeCreds(w, r)
	if !ok {
		return
	}
	token, err := h.svc.Login(r.Context(), creds.Login, creds.Password)
	if err != nil {
		h.handleLoginError(w, err)
		return
	}
	writeToken(w, token)
}

func decodeCreds(w http.ResponseWriter, r *http.Request) (credentials, bool) {
	var c credentials
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return c, false
	}
	if c.Login == "" || c.Password == "" {
		http.Error(w, "login and password are required", http.StatusBadRequest)
		return c, false
	}
	return c, true
}

func (h *Handler) handleRegisterError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, authsvc.ErrInvalidInput):
		http.Error(w, "bad request", http.StatusBadRequest)
	case errors.Is(err, domain.ErrLoginTaken):
		http.Error(w, "login taken", http.StatusConflict)
	default:
		h.log.Error("ошибка регистрации", logger.Err(err))
		http.Error(w, "internal", http.StatusInternalServerError)
	}
}

func (h *Handler) handleLoginError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials):
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
	default:
		h.log.Error("ошибка логина", logger.Err(err))
		http.Error(w, "internal", http.StatusInternalServerError)
	}
}

func writeToken(w http.ResponseWriter, token string) {
	w.Header().Set("Authorization", "Bearer "+token)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(authResponse{Token: token})
}
