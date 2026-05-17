package balance_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/domain"
	balancesvc "github.com/warenikov/gofermart/internal/service/balance"
)

type fakeWithdrawalRepo struct {
	getBalanceFn func(ctx context.Context, userID int64) (domain.Balance, error)
	withdrawFn   func(ctx context.Context, userID int64, orderNumber string, sum decimal.Decimal) error
	listByUserFn func(ctx context.Context, userID int64) ([]domain.Withdrawal, error)
}

func (f *fakeWithdrawalRepo) GetBalance(ctx context.Context, userID int64) (domain.Balance, error) {
	return f.getBalanceFn(ctx, userID)
}

func (f *fakeWithdrawalRepo) Withdraw(ctx context.Context, userID int64, orderNumber string, sum decimal.Decimal) error {
	return f.withdrawFn(ctx, userID, orderNumber, sum)
}

func (f *fakeWithdrawalRepo) ListByUser(ctx context.Context, userID int64) ([]domain.Withdrawal, error) {
	return f.listByUserFn(ctx, userID)
}

func TestService_Get_PassThrough(t *testing.T) {
	t.Parallel()
	want := domain.Balance{
		Current:   decimal.NewFromInt(100),
		Withdrawn: decimal.NewFromInt(50),
	}
	repo := &fakeWithdrawalRepo{getBalanceFn: func(_ context.Context, _ int64) (domain.Balance, error) {
		return want, nil
	}}
	svc := balancesvc.NewService(repo, zap.NewNop())

	got, err := svc.Get(t.Context(), 1)
	require.NoError(t, err)
	assert.True(t, got.Current.Equal(want.Current))
}

func TestService_Withdraw_NonPositiveSum_ReturnsInvalidSum(t *testing.T) {
	t.Parallel()
	svc := balancesvc.NewService(&fakeWithdrawalRepo{}, zap.NewNop())

	for _, s := range []decimal.Decimal{decimal.Zero, decimal.NewFromInt(-1)} {
		err := svc.Withdraw(t.Context(), 1, "12345678903", s)
		require.Error(t, err)
		assert.True(t, errors.Is(err, balancesvc.ErrInvalidSum), "sum=%s err=%v", s.String(), err)
	}
}

func TestService_Withdraw_InvalidLuhn(t *testing.T) {
	t.Parallel()
	svc := balancesvc.NewService(&fakeWithdrawalRepo{}, zap.NewNop())

	err := svc.Withdraw(t.Context(), 1, "12345678901", decimal.NewFromInt(10))
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidLuhn))
}

func TestService_Withdraw_PassesToRepo(t *testing.T) {
	t.Parallel()
	var savedOrder string
	var savedSum decimal.Decimal
	repo := &fakeWithdrawalRepo{withdrawFn: func(_ context.Context, _ int64, order string, sum decimal.Decimal) error {
		savedOrder = order
		savedSum = sum
		return nil
	}}
	svc := balancesvc.NewService(repo, zap.NewNop())

	err := svc.Withdraw(t.Context(), 1, "12345678903", decimal.NewFromInt(50))
	require.NoError(t, err)
	assert.Equal(t, "12345678903", savedOrder)
	assert.True(t, savedSum.Equal(decimal.NewFromInt(50)))
}

func TestService_Withdraw_PropagatesInsufficientFunds(t *testing.T) {
	t.Parallel()
	repo := &fakeWithdrawalRepo{withdrawFn: func(_ context.Context, _ int64, _ string, _ decimal.Decimal) error {
		return domain.ErrInsufficientFunds
	}}
	svc := balancesvc.NewService(repo, zap.NewNop())

	err := svc.Withdraw(t.Context(), 1, "12345678903", decimal.NewFromInt(50))
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInsufficientFunds))
}

func TestService_ListWithdrawals_PassThrough(t *testing.T) {
	t.Parallel()
	want := []domain.Withdrawal{{ID: 1, OrderNumber: "12345678903"}}
	repo := &fakeWithdrawalRepo{listByUserFn: func(_ context.Context, userID int64) ([]domain.Withdrawal, error) {
		assert.Equal(t, int64(7), userID)
		return want, nil
	}}
	svc := balancesvc.NewService(repo, zap.NewNop())

	got, err := svc.ListWithdrawals(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
