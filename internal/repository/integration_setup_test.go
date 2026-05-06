//go:build integration

package repository_test

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/warenikov/gofermart/internal/repository"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("DATABASE_URI")
	if dsn == "" {
		log.Println("DATABASE_URI не задан — интеграционные тесты пропущены")
		os.Exit(0)
	}

	if err := repository.ApplyMigrations(dsn); err != nil {
		log.Fatalf("ошибка миграций: %v", err)
	}

	pool, err := repository.NewPool(context.Background(), dsn)
	if err != nil {
		log.Fatalf("ошибка подключения к БД: %v", err)
	}
	testPool = pool

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// resetDB очищает все таблицы между тестами и сбрасывает sequence-ы.
func resetDB(t *testing.T) {
	t.Helper()
	const truncateAll = `TRUNCATE ` +
		repository.TableWithdrawals + `, ` +
		repository.TableOrders + `, ` +
		repository.TableUsers +
		` RESTART IDENTITY CASCADE`
	_, err := testPool.Exec(context.Background(), truncateAll)
	require.NoError(t, err)
}
