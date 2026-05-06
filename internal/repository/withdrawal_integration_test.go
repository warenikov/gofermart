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

func TestWithdrawalRepository_GetBalance_EmptyUser(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	repo := repository.NewWithdrawalRepository(testPool)

	balance, err := repo.GetBalance(context.Background(), userID)
	require.NoError(t, err)
	assert.True(t, balance.Current.IsZero())
	assert.True(t, balance.Withdrawn.IsZero())
}

func TestWithdrawalRepository_Withdraw_InsufficientFunds(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	repo := repository.NewWithdrawalRepository(testPool)

	err := repo.Withdraw(context.Background(), userID, "12345678903", decimal.NewFromInt(100))
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInsufficientFunds),
		"ожидается ErrInsufficientFunds, получено: %v", err)
}

func TestWithdrawalRepository_Withdraw_ReducesBalance(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	orderRepo := repository.NewOrderRepository(testPool)
	withRepo := repository.NewWithdrawalRepository(testPool)
	ctx := context.Background()

	require.NoError(t, orderRepo.Create(ctx, "12345678903", userID))
	require.NoError(t, orderRepo.UpdateStatus(ctx, "12345678903", domain.OrderStatusProcessed,
		decimal.NullDecimal{Decimal: decimal.NewFromInt(1000), Valid: true}))

	balance, err := withRepo.GetBalance(ctx, userID)
	require.NoError(t, err)
	assert.True(t, balance.Current.Equal(decimal.NewFromInt(1000)))

	require.NoError(t, withRepo.Withdraw(ctx, userID, "346436439", decimal.NewFromInt(300)))

	balance, err = withRepo.GetBalance(ctx, userID)
	require.NoError(t, err)
	assert.True(t, balance.Current.Equal(decimal.NewFromInt(700)),
		"ожидается 700, получено %s", balance.Current.String())
	assert.True(t, balance.Withdrawn.Equal(decimal.NewFromInt(300)))
}

func TestWithdrawalRepository_ListByUser(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	orderRepo := repository.NewOrderRepository(testPool)
	withRepo := repository.NewWithdrawalRepository(testPool)
	ctx := context.Background()

	require.NoError(t, orderRepo.Create(ctx, "12345678903", userID))
	require.NoError(t, orderRepo.UpdateStatus(ctx, "12345678903", domain.OrderStatusProcessed,
		decimal.NullDecimal{Decimal: decimal.NewFromInt(2000), Valid: true}))

	require.NoError(t, withRepo.Withdraw(ctx, userID, "346436439", decimal.NewFromInt(100)))
	require.NoError(t, withRepo.Withdraw(ctx, userID, "100000000008", decimal.NewFromInt(200)))

	list, err := withRepo.ListByUser(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestWithdrawalRepository_ListByUser_Empty(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	repo := repository.NewWithdrawalRepository(testPool)

	list, err := repo.ListByUser(context.Background(), userID)
	require.NoError(t, err)
	assert.Empty(t, list)
}
