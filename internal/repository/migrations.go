// Package repository содержит реализации репозиториев на pgx/v5 и встроенные SQL-миграции.
package repository

import (
	"embed"
	"errors"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // регистрирует драйвер postgres для migrate
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/samber/oops"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// applyMigrations прогоняет встроенные SQL-миграции до самой свежей версии.
// Идемпотентна: ErrNoChange трактуется как успех. Вызывается из [NewPool].
func applyMigrations(dsn string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return oops.In("repository.migrations").Code("iofs_open").Wrapf(err, "открыть встроенные миграции")
	}
	defer func() { _ = src.Close() }()

	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return oops.In("repository.migrations").Code("migrate_init").Wrapf(err, "инициализировать migrate")
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return oops.In("repository.migrations").Code("migrate_up").Wrapf(err, "применить миграции")
	}
	return nil
}
