package accrual

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/domain"
)

type fakeClient struct {
	getOrderFn func(ctx context.Context, number string) (*OrderInfo, error)
	calls      atomic.Int32
}

func (f *fakeClient) GetOrder(ctx context.Context, number string) (*OrderInfo, error) {
	f.calls.Add(1)
	return f.getOrderFn(ctx, number)
}

type fakeOrders struct {
	mu             sync.Mutex
	pending        []domain.Order
	updates        []update
	listUnfinished func(ctx context.Context, limit int) ([]domain.Order, error)
}

type update struct {
	number  string
	status  domain.OrderStatus
	accrual decimal.NullDecimal
}

func (f *fakeOrders) ListUnfinished(ctx context.Context, limit int) ([]domain.Order, error) {
	if f.listUnfinished != nil {
		return f.listUnfinished(ctx, limit)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return nil, nil
	}
	out := make([]domain.Order, len(f.pending))
	copy(out, f.pending)
	f.pending = nil
	return out, nil
}

func (f *fakeOrders) UpdateStatus(_ context.Context, number string, status domain.OrderStatus, accrual decimal.NullDecimal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, update{number: number, status: status, accrual: accrual})
	return nil
}

func TestProcessOrder_MapsProcessedWithAccrual(t *testing.T) {
	t.Parallel()
	client := &fakeClient{getOrderFn: func(_ context.Context, _ string) (*OrderInfo, error) {
		return &OrderInfo{
			Order:   "12345678903",
			Status:  StatusProcessed,
			Accrual: decimal.NullDecimal{Decimal: decimal.NewFromInt(500), Valid: true},
		}, nil
	}}
	orders := &fakeOrders{}

	rl, err := processOrder(t.Context(), client, orders, "12345678903")
	require.NoError(t, err)
	assert.Nil(t, rl)
	require.Len(t, orders.updates, 1)
	assert.Equal(t, domain.OrderStatusProcessed, orders.updates[0].status)
	assert.True(t, orders.updates[0].accrual.Valid)
}

func TestProcessOrder_MapsRegisteredToProcessing(t *testing.T) {
	t.Parallel()
	client := &fakeClient{getOrderFn: func(_ context.Context, _ string) (*OrderInfo, error) {
		return &OrderInfo{Order: "x", Status: StatusRegistered}, nil
	}}
	orders := &fakeOrders{}
	_, err := processOrder(t.Context(), client, orders, "x")
	require.NoError(t, err)
	require.Len(t, orders.updates, 1)
	assert.Equal(t, domain.OrderStatusProcessing, orders.updates[0].status)
}

func TestProcessOrder_NotRegistered_SkipsUpdate(t *testing.T) {
	t.Parallel()
	client := &fakeClient{getOrderFn: func(_ context.Context, _ string) (*OrderInfo, error) {
		return nil, ErrOrderNotRegistered
	}}
	orders := &fakeOrders{}
	rl, err := processOrder(t.Context(), client, orders, "x")
	require.NoError(t, err)
	assert.Nil(t, rl)
	assert.Empty(t, orders.updates, "не должно быть обновлений")
}

func TestProcessOrder_RateLimit_ReturnsRetryAfter(t *testing.T) {
	t.Parallel()
	client := &fakeClient{getOrderFn: func(_ context.Context, _ string) (*OrderInfo, error) {
		return nil, &RateLimitError{RetryAfter: 30 * time.Second}
	}}
	orders := &fakeOrders{}
	rl, err := processOrder(t.Context(), client, orders, "x")
	require.NoError(t, err)
	require.NotNil(t, rl)
	assert.Equal(t, 30*time.Second, rl.RetryAfter)
	assert.Empty(t, orders.updates)
}

func TestProcessOrder_TransportError_Propagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("network down")
	client := &fakeClient{getOrderFn: func(_ context.Context, _ string) (*OrderInfo, error) {
		return nil, boom
	}}
	rl, err := processOrder(t.Context(), client, &fakeOrders{}, "x")
	require.Error(t, err)
	assert.Nil(t, rl)
	assert.ErrorIs(t, err, boom)
}

func TestWorker_Run_ProcessesAndShutsDownOnCtxCancel(t *testing.T) {
	t.Parallel()
	orders := &fakeOrders{
		pending: []domain.Order{
			{Number: "12345678903", Status: domain.OrderStatusNew},
			{Number: "346436439", Status: domain.OrderStatusNew},
		},
	}
	client := &fakeClient{getOrderFn: func(_ context.Context, number string) (*OrderInfo, error) {
		return &OrderInfo{
			Order:   number,
			Status:  StatusProcessed,
			Accrual: decimal.NullDecimal{Decimal: decimal.NewFromInt(100), Valid: true},
		}, nil
	}}

	w := NewWorker(client, orders, WorkerConfig{
		PollInterval: 5 * time.Millisecond,
		Workers:      2,
		BatchLimit:   10,
	}, zap.NewNop())

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	require.Eventually(t, func() bool {
		orders.mu.Lock()
		defer orders.mu.Unlock()
		return len(orders.updates) == 2
	}, time.Second, 10*time.Millisecond, "оба заказа должны быть обновлены")

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершился после cancel")
	}
}

func TestWorker_Run_RateLimit_PausesGenerator(t *testing.T) {
	t.Parallel()
	var callCount atomic.Int32
	orders := &fakeOrders{
		listUnfinished: func(_ context.Context, _ int) ([]domain.Order, error) {
			n := callCount.Add(1)
			if n > 1 {
				// после первого вызова должно быть rate-limit, последующие
				// тики пропускаются — нового вызова почти не будет
				return nil, nil
			}
			return []domain.Order{{Number: "12345678903"}}, nil
		},
	}
	client := &fakeClient{getOrderFn: func(_ context.Context, _ string) (*OrderInfo, error) {
		return nil, &RateLimitError{RetryAfter: time.Hour}
	}}

	w := NewWorker(client, orders, WorkerConfig{
		PollInterval: 5 * time.Millisecond,
		Workers:      1,
		BatchLimit:   10,
	}, zap.NewNop())

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	time.Sleep(80 * time.Millisecond)
	cancel()
	<-done

	// без rate-limit мы бы видели десятки вызовов ListUnfinished за 80мс при тике 5мс.
	// С rate-limit — практически только первый.
	assert.Less(t, callCount.Load(), int32(5),
		"generator должен почти полностью встать после rate-limit, получено вызовов: %d",
		callCount.Load())
}
