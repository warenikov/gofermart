package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samber/oops"
)

// NewPool создаёт и пингует пул соединений pgx/v5.
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

	return pool, nil
}
