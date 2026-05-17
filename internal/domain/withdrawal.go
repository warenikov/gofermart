package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

// Withdrawal — операция списания баллов в счёт оплаты заказа.
type Withdrawal struct {
	ID          int64
	UserID      int64
	OrderNumber string
	Sum         decimal.Decimal
	ProcessedAt time.Time
}

// Balance — текущий баланс пользователя и сумма всех списаний.
type Balance struct {
	Current   decimal.Decimal
	Withdrawn decimal.Decimal
}
