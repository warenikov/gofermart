// Package order содержит HTTP-хендлеры заказов пользователя.
package order

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/domain"
	"github.com/warenikov/gofermart/internal/logger"
	"github.com/warenikov/gofermart/internal/middleware"
)

// Service — поведение, нужное хендлерам orders.
type Service interface {
	Submit(ctx context.Context, userID int64, number string) error
	List(ctx context.Context, userID int64) ([]domain.Order, error)
}

// Handler — HTTP-обвязка над сервисом orders.
type Handler struct {
	svc Service
	log *zap.Logger
}

// NewHandler создаёт хендлер.
func NewHandler(svc Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: logger.For(log, "handler.order")}
}

const maxBodyBytes = 1024

// Submit обрабатывает POST /api/user/orders.
// Body — text/plain c номером заказа.
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	number := strings.TrimSpace(string(body))
	if number == "" {
		http.Error(w, "empty order number", http.StatusBadRequest)
		return
	}

	switch err := h.svc.Submit(r.Context(), userID, number); {
	case err == nil:
		w.WriteHeader(http.StatusAccepted)
	case errors.Is(err, domain.ErrOrderAlreadyOwned):
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, domain.ErrOrderOwnedByOther):
		http.Error(w, "order owned by another user", http.StatusConflict)
	case errors.Is(err, domain.ErrInvalidLuhn):
		http.Error(w, "invalid order number", http.StatusUnprocessableEntity)
	default:
		h.log.Error("ошибка загрузки заказа", logger.Err(err))
		http.Error(w, "internal", http.StatusInternalServerError)
	}
}

// List обрабатывает GET /api/user/orders.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	orders, err := h.svc.List(r.Context(), userID)
	if err != nil {
		h.log.Error("ошибка списка заказов", logger.Err(err))
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	out := make([]orderDTO, len(orders))
	for i := range orders {
		out[i] = toOrderDTO(&orders[i])
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

type orderDTO struct {
	Number     string       `json:"number"`
	Status     string       `json:"status"`
	Accrual    *decimalJSON `json:"accrual,omitempty"`
	UploadedAt time.Time    `json:"uploaded_at"`
}

func toOrderDTO(o *domain.Order) orderDTO {
	dto := orderDTO{
		Number:     o.Number,
		Status:     string(o.Status),
		UploadedAt: o.UploadedAt,
	}
	if o.Accrual.Valid {
		d := decimalJSON(o.Accrual.Decimal)
		dto.Accrual = &d
	}
	return dto
}

// decimalJSON сериализует shopspring/decimal как JSON-число (без кавычек).
type decimalJSON decimal.Decimal

// MarshalJSON — числовое представление.
func (d decimalJSON) MarshalJSON() ([]byte, error) {
	return []byte(decimal.Decimal(d).String()), nil
}
