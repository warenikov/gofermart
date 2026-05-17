package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samber/oops"
	"github.com/shopspring/decimal"

	"github.com/warenikov/gofermart/internal/domain"
)

// OrderRepository — реализация domain.OrderRepository поверх PostgreSQL.
type OrderRepository struct {
	pool *pgxpool.Pool
}

// NewOrderRepository создаёт репозиторий заказов.
func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

func (r *OrderRepository) errIn(code string) oops.OopsErrorBuilder {
	return oops.In("repository.order").Code(code).Tags("postgres")
}

// Create регистрирует заказ за пользователем.
// Если заказ уже существует — возвращает ErrOrderAlreadyOwned (тот же пользователь) или ErrOrderOwnedByOther.
func (r *OrderRepository) Create(ctx context.Context, number string, userID int64) error {
	const q = `INSERT INTO ` + tableOrders + ` (number, user_id) VALUES ($1, $2)`

	_, err := r.pool.Exec(ctx, q, number, userID)
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		return r.resolveExistingOwner(ctx, number, userID)
	}
	return r.errIn("create").With("order_number", number).With("user_id", userID).
		Wrapf(err, "сохранить заказ")
}

func (r *OrderRepository) resolveExistingOwner(ctx context.Context, number string, userID int64) error {
	const q = `SELECT user_id FROM ` + tableOrders + ` WHERE number = $1`

	var existingOwner int64
	if err := r.pool.QueryRow(ctx, q, number).Scan(&existingOwner); err != nil {
		return r.errIn("resolve_owner").With("order_number", number).
			Wrapf(err, "определить владельца существующего заказа")
	}
	if existingOwner == userID {
		return r.errIn("already_owned").With("order_number", number).Wrap(domain.ErrOrderAlreadyOwned)
	}
	return r.errIn("owned_by_other").With("order_number", number).Wrap(domain.ErrOrderOwnedByOther)
}

// GetByNumber возвращает заказ по номеру.
func (r *OrderRepository) GetByNumber(ctx context.Context, number string) (*domain.Order, error) {
	const q = `SELECT number, user_id, status, accrual, uploaded_at FROM ` + tableOrders + ` WHERE number = $1`

	var o domain.Order
	err := r.pool.QueryRow(ctx, q, number).
		Scan(&o.Number, &o.UserID, &o.Status, &o.Accrual, &o.UploadedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, r.errIn("not_found").With("order_number", number).Wrap(domain.ErrOrderNotFound)
		}
		return nil, r.errIn("get_by_number").With("order_number", number).
			Wrapf(err, "получить заказ по номеру")
	}
	return &o, nil
}

// ListByUser возвращает все заказы пользователя в порядке от свежих к старым.
func (r *OrderRepository) ListByUser(ctx context.Context, userID int64) ([]domain.Order, error) {
	const q = `
		SELECT number, user_id, status, accrual, uploaded_at
		FROM ` + tableOrders + `
		WHERE user_id = $1
		ORDER BY uploaded_at DESC`

	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, r.errIn("list_by_user").With("user_id", userID).
			Wrapf(err, "получить заказы пользователя")
	}
	defer rows.Close()

	var out []domain.Order
	for rows.Next() {
		var o domain.Order
		if err := rows.Scan(&o.Number, &o.UserID, &o.Status, &o.Accrual, &o.UploadedAt); err != nil {
			return nil, r.errIn("list_by_user_scan").With("user_id", userID).
				Wrapf(err, "сканировать заказ")
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, r.errIn("list_by_user_rows").With("user_id", userID).
			Wrapf(err, "обойти заказы")
	}
	return out, nil
}

// UpdateStatus меняет статус и сумму начисления.
func (r *OrderRepository) UpdateStatus(
	ctx context.Context,
	number string,
	status domain.OrderStatus,
	accrual decimal.NullDecimal,
) error {
	const q = `UPDATE ` + tableOrders + ` SET status = $1, accrual = $2 WHERE number = $3`

	tag, err := r.pool.Exec(ctx, q, status, accrual, number)
	if err != nil {
		return r.errIn("update_status").With("order_number", number).With("status", string(status)).
			Wrapf(err, "обновить статус заказа")
	}
	if tag.RowsAffected() == 0 {
		return r.errIn("update_status_missing").With("order_number", number).Wrap(domain.ErrOrderNotFound)
	}
	return nil
}

// ListUnfinished возвращает заказы в статусах NEW и PROCESSING — для фонового поллера accrual.
func (r *OrderRepository) ListUnfinished(ctx context.Context, limit int) ([]domain.Order, error) {
	const q = `
		SELECT number, user_id, status, accrual, uploaded_at
		FROM ` + tableOrders + `
		WHERE status IN ('NEW', 'PROCESSING')
		ORDER BY uploaded_at ASC
		LIMIT $1`

	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, r.errIn("list_unfinished").With("limit", limit).
			Wrapf(err, "выбрать незавершённые заказы")
	}
	defer rows.Close()

	var out []domain.Order
	for rows.Next() {
		var o domain.Order
		if err := rows.Scan(&o.Number, &o.UserID, &o.Status, &o.Accrual, &o.UploadedAt); err != nil {
			return nil, r.errIn("list_unfinished_scan").Wrapf(err, "сканировать незавершённый заказ")
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, r.errIn("list_unfinished_rows").Wrapf(err, "обойти незавершённые заказы")
	}
	return out, nil
}
