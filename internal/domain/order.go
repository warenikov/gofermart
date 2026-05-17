package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

// OrderStatus — статус расчёта начисления по заказу.
type OrderStatus string

const (
	// OrderStatusNew — заказ загружен, расчёт ещё не запускался.
	OrderStatusNew OrderStatus = "NEW"
	// OrderStatusProcessing — расчёт начисления в процессе.
	OrderStatusProcessing OrderStatus = "PROCESSING"
	// OrderStatusInvalid — система начислений отвергла заказ.
	OrderStatusInvalid OrderStatus = "INVALID"
	// OrderStatusProcessed — расчёт успешно завершён, известна сумма начисления.
	OrderStatusProcessed OrderStatus = "PROCESSED"
)

// Order — загруженный пользователем заказ и его статус начисления.
type Order struct {
	Number     string
	UserID     int64
	Status     OrderStatus
	Accrual    decimal.NullDecimal
	UploadedAt time.Time
}
