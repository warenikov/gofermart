// Package order — сервисный слой для управления заказами пользователя.
package order

import (
	"context"

	"github.com/samber/oops"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/domain"
	"github.com/warenikov/gofermart/internal/logger"
)

// Service — бизнес-логика заказов.
type Service struct {
	repo domain.OrderRepository
	log  *zap.Logger
}

// NewService создаёт сервис.
func NewService(repo domain.OrderRepository, log *zap.Logger) *Service {
	return &Service{repo: repo, log: logger.For(log, "service.order")}
}

// Submit регистрирует заказ за пользователем.
// Возможные доменные ошибки: ErrInvalidLuhn, ErrOrderAlreadyOwned, ErrOrderOwnedByOther.
func (s *Service) Submit(ctx context.Context, userID int64, number string) error {
	if !domain.ValidateLuhn(number) {
		return oops.In("service.order").Code("invalid_luhn").
			With("number", number).Wrap(domain.ErrInvalidLuhn)
	}
	return s.repo.Create(ctx, number, userID)
}

// List возвращает все заказы пользователя.
func (s *Service) List(ctx context.Context, userID int64) ([]domain.Order, error) {
	return s.repo.ListByUser(ctx, userID)
}
