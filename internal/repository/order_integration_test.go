//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/warenikov/gofermart/internal/domain"
	"github.com/warenikov/gofermart/internal/repository"
)

func createTestUser(t *testing.T, login string) int64 {
	t.Helper()
	repo := repository.NewUserRepository(testPool)
	u, err := repo.Create(context.Background(), login, "hash")
	require.NoError(t, err)
	return u.ID
}

func TestOrderRepository_Create_GetByNumber(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	repo := repository.NewOrderRepository(testPool)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, "12345678903", userID))

	got, err := repo.GetByNumber(ctx, "12345678903")
	require.NoError(t, err)
	assert.Equal(t, "12345678903", got.Number)
	assert.Equal(t, userID, got.UserID)
	assert.Equal(t, domain.OrderStatusNew, got.Status)
	assert.False(t, got.Accrual.Valid, "accrual должен быть NULL для нового заказа")
	assert.False(t, got.UploadedAt.IsZero())
}

func TestOrderRepository_Create_DuplicateSameUser(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	repo := repository.NewOrderRepository(testPool)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, "12345678903", userID))

	err := repo.Create(ctx, "12345678903", userID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrOrderAlreadyOwned),
		"ожидается ErrOrderAlreadyOwned, получено: %v", err)
}

func TestOrderRepository_Create_DuplicateDifferentUser(t *testing.T) {
	resetDB(t)
	userA := createTestUser(t, "alice")
	userB := createTestUser(t, "bob")
	repo := repository.NewOrderRepository(testPool)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, "12345678903", userA))

	err := repo.Create(ctx, "12345678903", userB)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrOrderOwnedByOther),
		"ожидается ErrOrderOwnedByOther, получено: %v", err)
}

func TestOrderRepository_ListByUser(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	repo := repository.NewOrderRepository(testPool)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, "12345678903", userID))
	require.NoError(t, repo.Create(ctx, "346436439", userID))

	orders, err := repo.ListByUser(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, orders, 2)
}

func TestOrderRepository_ListByUser_Empty(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	repo := repository.NewOrderRepository(testPool)

	orders, err := repo.ListByUser(context.Background(), userID)
	require.NoError(t, err)
	assert.Empty(t, orders)
}

func TestOrderRepository_UpdateStatus(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	repo := repository.NewOrderRepository(testPool)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, "12345678903", userID))

	accrual := decimal.NullDecimal{Decimal: decimal.NewFromInt(500), Valid: true}
	require.NoError(t, repo.UpdateStatus(ctx, "12345678903", domain.OrderStatusProcessed, accrual))

	got, err := repo.GetByNumber(ctx, "12345678903")
	require.NoError(t, err)
	assert.Equal(t, domain.OrderStatusProcessed, got.Status)
	require.True(t, got.Accrual.Valid)
	assert.True(t, got.Accrual.Decimal.Equal(decimal.NewFromInt(500)))
}

func TestOrderRepository_UpdateStatus_NotFound(t *testing.T) {
	resetDB(t)
	repo := repository.NewOrderRepository(testPool)

	err := repo.UpdateStatus(
		context.Background(),
		"99999999999",
		domain.OrderStatusInvalid,
		decimal.NullDecimal{},
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrOrderNotFound))
}

func TestOrderRepository_ListUnfinished(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	repo := repository.NewOrderRepository(testPool)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, "12345678903", userID))
	require.NoError(t, repo.Create(ctx, "346436439", userID))
	require.NoError(t, repo.UpdateStatus(ctx, "346436439", domain.OrderStatusProcessed,
		decimal.NullDecimal{Decimal: decimal.NewFromInt(100), Valid: true}))

	unfinished, err := repo.ListUnfinished(ctx, 10)
	require.NoError(t, err)
	require.Len(t, unfinished, 1)
	assert.Equal(t, "12345678903", unfinished[0].Number)
}

func TestOrderRepository_GetByNumber_NotFound(t *testing.T) {
	resetDB(t)
	repo := repository.NewOrderRepository(testPool)

	_, err := repo.GetByNumber(context.Background(), "00000000000")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrOrderNotFound))
}
