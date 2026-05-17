// Package balance содержит HTTP-хендлеры баланса и списаний пользователя.
package balance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/domain"
	"github.com/warenikov/gofermart/internal/logger"
	"github.com/warenikov/gofermart/internal/middleware"
	balancesvc "github.com/warenikov/gofermart/internal/service/balance"
)

// Service — поведение, нужное хендлерам balance.
type Service interface {
	Get(ctx context.Context, userID int64) (domain.Balance, error)
	Withdraw(ctx context.Context, userID int64, orderNumber string, sum decimal.Decimal) error
	ListWithdrawals(ctx context.Context, userID int64) ([]domain.Withdrawal, error)
}

// Handler — HTTP-обвязка над сервисом balance.
type Handler struct {
	svc Service
	log *zap.Logger
}

// NewHandler создаёт хендлер.
func NewHandler(svc Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: logger.For(log, "handler.balance")}
}

// Get обрабатывает GET /api/user/balance.
//
//	@Summary	Текущий баланс пользователя
//	@Tags		balance
//	@Produce	json
//	@Success	200	{object}	balanceDTO	"Текущий баланс и сумма всех списаний"
//	@Failure	401	"Пользователь не авторизован"
//	@Failure	500	"Внутренняя ошибка сервера"
//	@Security	BearerAuth
//	@Router		/api/user/balance [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	b, err := h.svc.Get(r.Context(), userID)
	if err != nil {
		h.log.Error("ошибка получения баланса", logger.Err(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(balanceDTO{
		Current:   decimalJSON(b.Current),
		Withdrawn: decimalJSON(b.Withdrawn),
	})
}

// Withdraw обрабатывает POST /api/user/balance/withdraw.
//
//	@Summary	Списание баллов в счёт оплаты заказа
//	@Tags		balance
//	@Accept		json
//	@Param		withdraw	body	withdrawRequest	true	"Номер заказа и сумма к списанию"
//	@Success	200			"Списание зафиксировано"
//	@Failure	400			"Неверный формат запроса"
//	@Failure	401			"Пользователь не авторизован"
//	@Failure	402			"Недостаточно средств на счёте"
//	@Failure	422			"Неверный номер заказа (Луна)"
//	@Failure	500			"Внутренняя ошибка сервера"
//	@Security	BearerAuth
//	@Router		/api/user/balance/withdraw [post]
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	var req withdrawRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if req.Order == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	switch err := h.svc.Withdraw(r.Context(), userID, req.Order, req.Sum); {
	case err == nil:
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, balancesvc.ErrInvalidSum):
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	case errors.Is(err, domain.ErrInvalidLuhn):
		http.Error(w, http.StatusText(http.StatusUnprocessableEntity), http.StatusUnprocessableEntity)
	case errors.Is(err, domain.ErrInsufficientFunds):
		http.Error(w, http.StatusText(http.StatusPaymentRequired), http.StatusPaymentRequired)
	default:
		h.log.Error("ошибка списания", logger.Err(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// ListWithdrawals обрабатывает GET /api/user/withdrawals.
//
//	@Summary	Список списаний пользователя
//	@Tags		balance
//	@Produce	json
//	@Success	200	{array}	withdrawalDTO	"Список списаний в порядке от свежих к старым"
//	@Success	204	"Нет ни одного списания"
//	@Failure	401	"Пользователь не авторизован"
//	@Failure	500	"Внутренняя ошибка сервера"
//	@Security	BearerAuth
//	@Router		/api/user/withdrawals [get]
func (h *Handler) ListWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	list, err := h.svc.ListWithdrawals(r.Context(), userID)
	if err != nil {
		h.log.Error("ошибка списка списаний", logger.Err(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if len(list) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	out := make([]withdrawalDTO, len(list))
	for i, wth := range list {
		out[i] = withdrawalDTO{
			Order:       wth.OrderNumber,
			Sum:         decimalJSON(wth.Sum),
			ProcessedAt: wth.ProcessedAt,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

type balanceDTO struct {
	Current   decimalJSON `json:"current"`
	Withdrawn decimalJSON `json:"withdrawn"`
}

type withdrawRequest struct {
	Order string          `json:"order"`
	Sum   decimal.Decimal `json:"sum"`
}

type withdrawalDTO struct {
	Order       string      `json:"order"`
	Sum         decimalJSON `json:"sum"`
	ProcessedAt time.Time   `json:"processed_at"`
}

// decimalJSON сериализует shopspring/decimal как JSON-число.
type decimalJSON decimal.Decimal

// MarshalJSON — числовое представление.
func (d decimalJSON) MarshalJSON() ([]byte, error) {
	return []byte(decimal.Decimal(d).String()), nil
}
