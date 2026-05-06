package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samber/oops"

	"github.com/warenikov/gofermart/internal/domain"
)

// UserRepository — реализация domain.UserRepository поверх PostgreSQL.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository создаёт репозиторий пользователей.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) errIn(code string) oops.OopsErrorBuilder {
	return oops.In("repository.user").Code(code).Tags("postgres")
}

// Create сохраняет нового пользователя.
func (r *UserRepository) Create(ctx context.Context, login, passwordHash string) (*domain.User, error) {
	const q = `
		INSERT INTO ` + TableUsers + ` (login, password_hash)
		VALUES ($1, $2)
		RETURNING id, login, password_hash, created_at`

	var u domain.User
	err := r.pool.QueryRow(ctx, q, login, passwordHash).
		Scan(&u.ID, &u.Login, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return nil, r.errIn("login_taken").With("login", login).Wrap(domain.ErrLoginTaken)
		}
		return nil, r.errIn("create").With("login", login).Wrapf(err, "сохранить пользователя")
	}
	return &u, nil
}

// GetByLogin возвращает пользователя по логину.
func (r *UserRepository) GetByLogin(ctx context.Context, login string) (*domain.User, error) {
	const q = `SELECT id, login, password_hash, created_at FROM ` + TableUsers + ` WHERE login = $1`

	var u domain.User
	err := r.pool.QueryRow(ctx, q, login).
		Scan(&u.ID, &u.Login, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, r.errIn("not_found").With("login", login).Wrap(domain.ErrUserNotFound)
		}
		return nil, r.errIn("get_by_login").With("login", login).Wrapf(err, "получить пользователя по логину")
	}
	return &u, nil
}

// GetByID возвращает пользователя по идентификатору.
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	const q = `SELECT id, login, password_hash, created_at FROM ` + TableUsers + ` WHERE id = $1`

	var u domain.User
	err := r.pool.QueryRow(ctx, q, id).
		Scan(&u.ID, &u.Login, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, r.errIn("not_found").With("user_id", id).Wrap(domain.ErrUserNotFound)
		}
		return nil, r.errIn("get_by_id").With("user_id", id).Wrapf(err, "получить пользователя по id")
	}
	return &u, nil
}
