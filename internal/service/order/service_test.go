package order_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/domain"
	ordersvc "github.com/warenikov/gofermart/internal/service/order"
)

type fakeOrderRepo struct {
	createFn         func(ctx context.Context, number string, userID int64) error
	getByNumberFn    func(ctx context.Context, number string) (*domain.Order, error)
	listByUserFn     func(ctx context.Context, userID int64) ([]domain.Order, error)
	updateStatusFn   func(ctx context.Context, number string, status domain.OrderStatus, accrual decimal.NullDecimal) error
	listUnfinishedFn func(ctx context.Context, limit int) ([]domain.Order, error)
}

func (f *fakeOrderRepo) Create(ctx context.Context, number string, userID int64) error {
	return f.createFn(ctx, number, userID)
}

func (f *fakeOrderRepo) GetByNumber(ctx context.Context, number string) (*domain.Order, error) {
	return f.getByNumberFn(ctx, number)
}

func (f *fakeOrderRepo) ListByUser(ctx context.Context, userID int64) ([]domain.Order, error) {
	return f.listByUserFn(ctx, userID)
}

func (f *fakeOrderRepo) UpdateStatus(ctx context.Context, number string, status domain.OrderStatus, accrual decimal.NullDecimal) error {
	return f.updateStatusFn(ctx, number, status, accrual)
}

func (f *fakeOrderRepo) ListUnfinished(ctx context.Context, limit int) ([]domain.Order, error) {
	return f.listUnfinishedFn(ctx, limit)
}

func TestService_Submit_InvalidLuhn(t *testing.T) {
	t.Parallel()
	svc := ordersvc.NewService(&fakeOrderRepo{}, zap.NewNop())

	cases := []string{"", "12345678901", "abc", "12345 78903"}
	for _, c := range cases {
		err := svc.Submit(t.Context(), 1, c)
		require.Error(t, err, "input=%q", c)
		assert.True(t, errors.Is(err, domain.ErrInvalidLuhn), "input=%q err=%v", c, err)
	}
}

func TestService_Submit_PassesValidNumberToRepo(t *testing.T) {
	t.Parallel()
	var savedNumber string
	var savedUser int64
	repo := &fakeOrderRepo{createFn: func(_ context.Context, number string, userID int64) error {
		savedNumber = number
		savedUser = userID
		return nil
	}}
	svc := ordersvc.NewService(repo, zap.NewNop())

	err := svc.Submit(t.Context(), 7, "12345678903")
	require.NoError(t, err)
	assert.Equal(t, "12345678903", savedNumber)
	assert.Equal(t, int64(7), savedUser)
}

func TestService_Submit_PropagatesAlreadyOwned(t *testing.T) {
	t.Parallel()
	repo := &fakeOrderRepo{createFn: func(_ context.Context, _ string, _ int64) error {
		return domain.ErrOrderAlreadyOwned
	}}
	svc := ordersvc.NewService(repo, zap.NewNop())

	err := svc.Submit(t.Context(), 1, "12345678903")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrOrderAlreadyOwned))
}

func TestService_Submit_PropagatesOwnedByOther(t *testing.T) {
	t.Parallel()
	repo := &fakeOrderRepo{createFn: func(_ context.Context, _ string, _ int64) error {
		return domain.ErrOrderOwnedByOther
	}}
	svc := ordersvc.NewService(repo, zap.NewNop())

	err := svc.Submit(t.Context(), 1, "12345678903")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrOrderOwnedByOther))
}

func TestService_List_PassThrough(t *testing.T) {
	t.Parallel()
	want := []domain.Order{{Number: "12345678903"}}
	repo := &fakeOrderRepo{listByUserFn: func(_ context.Context, userID int64) ([]domain.Order, error) {
		assert.Equal(t, int64(5), userID)
		return want, nil
	}}
	svc := ordersvc.NewService(repo, zap.NewNop())

	got, err := svc.List(t.Context(), 5)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
