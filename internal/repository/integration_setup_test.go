//go:build integration

package repository

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("DATABASE_URI")
	if dsn == "" {
		log.Println("DATABASE_URI не задан — интеграционные тесты пропущены")
		os.Exit(0)
	}

	pool, err := NewPool(context.Background(), dsn)
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
		tableWithdrawals + `, ` +
		tableOrders + `, ` +
		tableUsers +
		` RESTART IDENTITY CASCADE`
	_, err := testPool.Exec(context.Background(), truncateAll)
	require.NoError(t, err)
}
