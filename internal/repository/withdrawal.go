package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samber/oops"
	"github.com/shopspring/decimal"

	"github.com/warenikov/gofermart/internal/domain"
)

// WithdrawalRepository — реализация domain.WithdrawalRepository поверх PostgreSQL.
type WithdrawalRepository struct {
	pool *pgxpool.Pool
}

// NewWithdrawalRepository создаёт репозиторий списаний.
func NewWithdrawalRepository(pool *pgxpool.Pool) *WithdrawalRepository {
	return &WithdrawalRepository{pool: pool}
}

func (r *WithdrawalRepository) errIn(code string) oops.OopsErrorBuilder {
	return oops.In("repository.withdrawal").Code(code).Tags("postgres")
}

// GetBalance возвращает текущий баланс и сумму всех списаний.
// Текущий баланс = сумма accrual по PROCESSED-заказам − сумма всех списаний.
func (r *WithdrawalRepository) GetBalance(ctx context.Context, userID int64) (domain.Balance, error) {
	const q = `
		SELECT
			COALESCE((SELECT SUM(accrual) FROM ` + TableOrders + `
				WHERE user_id = $1 AND status = 'PROCESSED'), 0) AS accrued,
			COALESCE((SELECT SUM(sum) FROM ` + TableWithdrawals + `
				WHERE user_id = $1), 0) AS withdrawn`

	var accrued, withdrawn decimal.Decimal
	if err := r.pool.QueryRow(ctx, q, userID).Scan(&accrued, &withdrawn); err != nil {
		return domain.Balance{}, r.errIn("get_balance").With("user_id", userID).
			Wrapf(err, "посчитать баланс пользователя")
	}
	return domain.Balance{
		Current:   accrued.Sub(withdrawn),
		Withdrawn: withdrawn,
	}, nil
}

// Withdraw атомарно проверяет баланс и регистрирует списание.
// Корректность не зависит от уровня изоляции: строка пользователя блокируется
// через SELECT ... FOR UPDATE на время транзакции — параллельные Withdraw
// на одного user_id сериализуются, на разных пользователей идут параллельно.
func (r *WithdrawalRepository) Withdraw(
	ctx context.Context,
	userID int64,
	orderNumber string,
	sum decimal.Decimal,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return r.errIn("tx_begin").With("user_id", userID).Wrapf(err, "открыть транзакцию списания")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const lockUserQ = `SELECT 1 FROM ` + TableUsers + ` WHERE id = $1 FOR UPDATE`
	var dummy int
	if err := tx.QueryRow(ctx, lockUserQ, userID).Scan(&dummy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return r.errIn("user_not_found").With("user_id", userID).Wrap(domain.ErrUserNotFound)
		}
		return r.errIn("lock_user").With("user_id", userID).Wrapf(err, "заблокировать строку пользователя")
	}

	const balanceQ = `
		SELECT
			COALESCE((SELECT SUM(accrual) FROM ` + TableOrders + `
				WHERE user_id = $1 AND status = 'PROCESSED'), 0)
			-
			COALESCE((SELECT SUM(sum) FROM ` + TableWithdrawals + `
				WHERE user_id = $1), 0) AS balance`

	var balance decimal.Decimal
	if err := tx.QueryRow(ctx, balanceQ, userID).Scan(&balance); err != nil {
		return r.errIn("balance_check").With("user_id", userID).
			Wrapf(err, "проверить баланс перед списанием")
	}

	if balance.LessThan(sum) {
		return r.errIn("insufficient_funds").
			With("user_id", userID).
			With("balance", balance.String()).
			With("requested", sum.String()).
			Wrap(domain.ErrInsufficientFunds)
	}

	const insertQ = `
		INSERT INTO ` + TableWithdrawals + ` (user_id, order_number, sum)
		VALUES ($1, $2, $3)`

	if _, err := tx.Exec(ctx, insertQ, userID, orderNumber, sum); err != nil {
		return r.errIn("insert").With("user_id", userID).With("order_number", orderNumber).
			Wrapf(err, "записать списание")
	}

	if err := tx.Commit(ctx); err != nil {
		return r.errIn("tx_commit").With("user_id", userID).
			Wrapf(err, "зафиксировать транзакцию списания")
	}
	return nil
}

// ListByUser возвращает все списания пользователя в порядке от свежих к старым.
func (r *WithdrawalRepository) ListByUser(ctx context.Context, userID int64) ([]domain.Withdrawal, error) {
	const q = `
		SELECT id, user_id, order_number, sum, processed_at
		FROM ` + TableWithdrawals + `
		WHERE user_id = $1
		ORDER BY processed_at DESC`

	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, r.errIn("list_by_user").With("user_id", userID).
			Wrapf(err, "получить списания пользователя")
	}
	defer rows.Close()

	var out []domain.Withdrawal
	for rows.Next() {
		var w domain.Withdrawal
		if err := rows.Scan(&w.ID, &w.UserID, &w.OrderNumber, &w.Sum, &w.ProcessedAt); err != nil {
			return nil, r.errIn("list_by_user_scan").With("user_id", userID).
				Wrapf(err, "сканировать списание")
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, r.errIn("list_by_user_rows").With("user_id", userID).
			Wrapf(err, "обойти списания")
	}
	return out, nil
}
