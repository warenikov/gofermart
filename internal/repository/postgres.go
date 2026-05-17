package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samber/oops"
)

// NewPool создаёт пул соединений pgx/v5, пингует БД и применяет встроенные миграции.
// Миграции применяются до возврата пула — потребитель получает БД, готовую к работе.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, oops.In("repository.postgres").Code("parse_dsn").Wrapf(err, "разобрать DSN")
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, oops.In("repository.postgres").Code("pool_init").Wrapf(err, "создать пул pgx")
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, oops.In("repository.postgres").Code("ping").Wrapf(err, "проверить доступность БД")
	}

	if err := applyMigrations(dsn); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
