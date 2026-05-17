//go:build integration

package repository

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/warenikov/gofermart/internal/domain"
)

func TestWithdrawalRepository_GetBalance_EmptyUser(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	repo := NewWithdrawalRepository(testPool)

	balance, err := repo.GetBalance(context.Background(), userID)
	require.NoError(t, err)
	assert.True(t, balance.Current.IsZero())
	assert.True(t, balance.Withdrawn.IsZero())
}

func TestWithdrawalRepository_Withdraw_InsufficientFunds(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	repo := NewWithdrawalRepository(testPool)

	err := repo.Withdraw(context.Background(), userID, "12345678903", decimal.NewFromInt(100))
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInsufficientFunds),
		"ожидается ErrInsufficientFunds, получено: %v", err)
}

func TestWithdrawalRepository_Withdraw_ReducesBalance(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	orderRepo := NewOrderRepository(testPool)
	withRepo := NewWithdrawalRepository(testPool)
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
	orderRepo := NewOrderRepository(testPool)
	withRepo := NewWithdrawalRepository(testPool)
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
	repo := NewWithdrawalRepository(testPool)

	list, err := repo.ListByUser(context.Background(), userID)
	require.NoError(t, err)
	assert.Empty(t, list)
}

// TestWithdrawalRepository_Withdraw_NoRaceOnSameUser проверяет, что параллельные
// Withdraw на одного пользователя при балансе ровно на одно списание дают
// ровно один успех и N−1 ErrInsufficientFunds — никакого двойного списания.
// Без блокировки строки пользователя (FOR UPDATE) на уровне READ COMMITTED
// такой тест поймал бы race и баланс ушёл в минус.
func TestWithdrawalRepository_Withdraw_NoRaceOnSameUser(t *testing.T) {
	resetDB(t)
	userID := createTestUser(t, "alice")
	orderRepo := NewOrderRepository(testPool)
	withRepo := NewWithdrawalRepository(testPool)
	ctx := context.Background()

	require.NoError(t, orderRepo.Create(ctx, "12345678903", userID))
	require.NoError(t, orderRepo.UpdateStatus(ctx, "12345678903", domain.OrderStatusProcessed,
		decimal.NullDecimal{Decimal: decimal.NewFromInt(100), Valid: true}))

	const goroutines = 10
	var (
		wg                sync.WaitGroup
		successes         atomic.Int32
		insufficient      atomic.Int32
		unexpectedErrors  atomic.Int32
		start             = make(chan struct{})
		withdrawalNumbers = []string{
			"346436439", "100000000008", "12345678903", "79927398713", "49927398716",
			"6011514433546201", "5555555555554444", "4111111111111111", "30569309025904", "18",
		}
	)

	for i := range goroutines {
		wg.Add(1)
		go func(orderNum string) {
			defer wg.Done()
			<-start
			err := withRepo.Withdraw(ctx, userID, orderNum, decimal.NewFromInt(100))
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, domain.ErrInsufficientFunds):
				insufficient.Add(1)
			default:
				unexpectedErrors.Add(1)
				t.Errorf("неожиданная ошибка от Withdraw: %v", err)
			}
		}(withdrawalNumbers[i])
	}

	close(start)
	wg.Wait()

	assert.Equal(t, int32(1), successes.Load(),
		"должно пройти ровно одно списание, прошло %d", successes.Load())
	assert.Equal(t, int32(goroutines-1), insufficient.Load(),
		"остальные %d должны вернуть ErrInsufficientFunds, вернули %d",
		goroutines-1, insufficient.Load())
	assert.Zero(t, unexpectedErrors.Load(), "не должно быть неожиданных ошибок")

	balance, err := withRepo.GetBalance(ctx, userID)
	require.NoError(t, err)
	assert.True(t, balance.Current.IsZero(),
		"итоговый баланс должен быть 0, получено %s", balance.Current.String())
	assert.True(t, balance.Withdrawn.Equal(decimal.NewFromInt(100)),
		"сумма списаний должна быть 100, получено %s", balance.Withdrawn.String())
}

func TestWithdrawalRepository_Withdraw_UserNotFound(t *testing.T) {
	resetDB(t)
	repo := NewWithdrawalRepository(testPool)

	err := repo.Withdraw(context.Background(), 99999, "12345678903", decimal.NewFromInt(50))
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrUserNotFound),
		"ожидается ErrUserNotFound, получено: %v", err)
}
