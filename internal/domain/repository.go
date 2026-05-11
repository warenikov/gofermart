package domain

import (
	"context"

	"github.com/shopspring/decimal"
)

// UserRepository — операции над учётными записями пользователей.
type UserRepository interface {
	// Create сохраняет нового пользователя. Возвращает ErrLoginTaken, если логин уже занят.
	Create(ctx context.Context, login, passwordHash string) (*User, error)
	// GetByLogin возвращает пользователя по логину. ErrUserNotFound, если не найден.
	GetByLogin(ctx context.Context, login string) (*User, error)
	// GetByID возвращает пользователя по идентификатору. ErrUserNotFound, если не найден.
	GetByID(ctx context.Context, id int64) (*User, error)
}

// OrderRepository — операции над заказами пользователей.
type OrderRepository interface {
	// Create регистрирует новый заказ за пользователем.
	// ErrOrderAlreadyOwned — заказ уже загружен этим пользователем.
	// ErrOrderOwnedByOther — заказ загружен другим пользователем.
	Create(ctx context.Context, number string, userID int64) error
	// GetByNumber возвращает заказ по номеру. ErrOrderNotFound, если не найден.
	GetByNumber(ctx context.Context, number string) (*Order, error)
	// ListByUser возвращает все заказы пользователя в порядке загрузки от свежих к старым.
	ListByUser(ctx context.Context, userID int64) ([]Order, error)
	// UpdateStatus меняет статус заказа и сумму начисления.
	UpdateStatus(ctx context.Context, number string, status OrderStatus, accrual decimal.NullDecimal) error
	// ListUnfinished возвращает заказы в статусах NEW и PROCESSING (для фонового поллера).
	ListUnfinished(ctx context.Context, limit int) ([]Order, error)
}

// WithdrawalRepository — операции над балансом и списаниями пользователя.
type WithdrawalRepository interface {
	// GetBalance возвращает текущий баланс и сумму всех списаний пользователя.
	GetBalance(ctx context.Context, userID int64) (Balance, error)
	// Withdraw атомарно проверяет баланс и регистрирует списание.
	// ErrInsufficientFunds — на счёте недостаточно средств.
	// ErrUserNotFound — пользователь был удалён между авторизацией и списанием.
	Withdraw(ctx context.Context, userID int64, orderNumber string, sum decimal.Decimal) error
	// ListByUser возвращает все списания пользователя в порядке от свежих к старым.
	ListByUser(ctx context.Context, userID int64) ([]Withdrawal, error)
}
