// Package balance — сервисный слой баланса и списаний.
package balance

import (
	"context"
	"errors"

	"github.com/samber/oops"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/domain"
	"github.com/warenikov/gofermart/internal/logger"
)

// ErrInvalidSum — сумма списания должна быть положительной.
var ErrInvalidSum = errors.New("withdrawal sum must be positive")

// Service — бизнес-логика баланса и списаний.
type Service struct {
	repo domain.WithdrawalRepository
	log  *zap.Logger
}

// NewService создаёт сервис.
func NewService(repo domain.WithdrawalRepository, log *zap.Logger) *Service {
	return &Service{repo: repo, log: logger.For(log, "service.balance")}
}

// Get возвращает текущий баланс и сумму всех списаний пользователя.
func (s *Service) Get(ctx context.Context, userID int64) (domain.Balance, error) {
	return s.repo.GetBalance(ctx, userID)
}

// Withdraw валидирует номер заказа по Луна и регистрирует списание.
// Возможные ошибки: ErrInvalidSum, domain.ErrInvalidLuhn, domain.ErrInsufficientFunds.
func (s *Service) Withdraw(ctx context.Context, userID int64, orderNumber string, sum decimal.Decimal) error {
	if !sum.IsPositive() {
		return oops.In("service.balance").Code("non_positive_sum").
			With("sum", sum.String()).Wrap(ErrInvalidSum)
	}
	if !domain.ValidateLuhn(orderNumber) {
		return oops.In("service.balance").Code("invalid_luhn").
			With("order_number", orderNumber).Wrap(domain.ErrInvalidLuhn)
	}
	return s.repo.Withdraw(ctx, userID, orderNumber, sum)
}

// ListWithdrawals возвращает все списания пользователя.
func (s *Service) ListWithdrawals(ctx context.Context, userID int64) ([]domain.Withdrawal, error) {
	return s.repo.ListByUser(ctx, userID)
}
