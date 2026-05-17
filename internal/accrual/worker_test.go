package accrual_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/accrual"
	mocks "github.com/warenikov/gofermart/internal/accrual/mocks"
	"github.com/warenikov/gofermart/internal/domain"
)

// fakeOrders — простой stateful-репозиторий для тестов воркера.
// AccrualClient мокается через mockery, а OrderUpdater остаётся
// stateful-фейком: моки testify плохо работают с потоковым накоплением событий.
type fakeOrders struct {
	mu             sync.Mutex
	pending        []domain.Order
	updates        []orderUpdate
	listUnfinished func(ctx context.Context, limit int) ([]domain.Order, error)
}

type orderUpdate struct {
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
	f.updates = append(f.updates, orderUpdate{number: number, status: status, accrual: accrual})
	return nil
}

func (f *fakeOrders) snapshotUpdates() []orderUpdate {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]orderUpdate, len(f.updates))
	copy(out, f.updates)
	return out
}

func newWorker(t *testing.T, client accrual.AccrualClient, orders accrual.OrderUpdater) *accrual.Worker {
	t.Helper()
	return accrual.NewWorker(client, orders, accrual.WorkerConfig{
		PollInterval: 5 * time.Millisecond,
		Workers:      2,
		BatchLimit:   10,
	}, zap.NewNop())
}

func runWorker(t *testing.T, w *accrual.Worker) (cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	d := make(chan error, 1)
	go func() { d <- w.Run(ctx) }()
	return cancel, d
}

func TestWorker_Run_ProcessedWithAccrual_UpdatesOrder(t *testing.T) {
	t.Parallel()
	client := mocks.NewMockAccrualClient(t)
	client.EXPECT().
		GetOrder(mock.Anything, "12345678903").
		Return(&accrual.OrderInfo{
			Order:   "12345678903",
			Status:  accrual.StatusProcessed,
			Accrual: decimal.NullDecimal{Decimal: decimal.NewFromInt(500), Valid: true},
		}, nil).
		Maybe()

	orders := &fakeOrders{pending: []domain.Order{{Number: "12345678903"}}}
	w := newWorker(t, client, orders)

	cancel, done := runWorker(t, w)
	defer cancel()

	require.Eventually(t, func() bool {
		return len(orders.snapshotUpdates()) == 1
	}, time.Second, 5*time.Millisecond)
	cancel()
	<-done

	upd := orders.snapshotUpdates()[0]
	assert.Equal(t, domain.OrderStatusProcessed, upd.status)
	require.True(t, upd.accrual.Valid)
	assert.True(t, upd.accrual.Decimal.Equal(decimal.NewFromInt(500)))
}

func TestWorker_Run_RegisteredStatus_MapsToProcessing(t *testing.T) {
	t.Parallel()
	client := mocks.NewMockAccrualClient(t)
	client.EXPECT().
		GetOrder(mock.Anything, mock.Anything).
		Return(&accrual.OrderInfo{Order: "x", Status: accrual.StatusRegistered}, nil).
		Maybe()

	orders := &fakeOrders{pending: []domain.Order{{Number: "x"}}}
	w := newWorker(t, client, orders)

	cancel, done := runWorker(t, w)
	defer cancel()
	require.Eventually(t, func() bool {
		return len(orders.snapshotUpdates()) == 1
	}, time.Second, 5*time.Millisecond)
	cancel()
	<-done

	assert.Equal(t, domain.OrderStatusProcessing, orders.snapshotUpdates()[0].status)
}

func TestWorker_Run_NotRegistered_SkipsUpdate(t *testing.T) {
	t.Parallel()
	client := mocks.NewMockAccrualClient(t)
	client.EXPECT().
		GetOrder(mock.Anything, mock.Anything).
		Return(nil, accrual.ErrOrderNotRegistered).
		Maybe()

	orders := &fakeOrders{pending: []domain.Order{{Number: "x"}}}
	w := newWorker(t, client, orders)

	cancel, done := runWorker(t, w)
	defer cancel()
	// Ждём, пока генератор успеет сходить за заказом, потом останавливаем.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	assert.Empty(t, orders.snapshotUpdates(), "204 не должно приводить к обновлению")
}

func TestWorker_Run_TransportError_DoesNotUpdate(t *testing.T) {
	t.Parallel()
	boom := errors.New("network down")
	client := mocks.NewMockAccrualClient(t)
	client.EXPECT().
		GetOrder(mock.Anything, mock.Anything).
		Return(nil, boom).
		Maybe()

	orders := &fakeOrders{pending: []domain.Order{{Number: "x"}}}
	w := newWorker(t, client, orders)

	cancel, done := runWorker(t, w)
	defer cancel()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	assert.Empty(t, orders.snapshotUpdates())
}

func TestWorker_Run_ProcessesAndShutsDownOnCtxCancel(t *testing.T) {
	t.Parallel()
	client := mocks.NewMockAccrualClient(t)
	client.EXPECT().
		GetOrder(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, number string) (*accrual.OrderInfo, error) {
			return &accrual.OrderInfo{
				Order:   number,
				Status:  accrual.StatusProcessed,
				Accrual: decimal.NullDecimal{Decimal: decimal.NewFromInt(100), Valid: true},
			}, nil
		}).
		Maybe()

	orders := &fakeOrders{pending: []domain.Order{
		{Number: "12345678903"},
		{Number: "346436439"},
	}}
	w := newWorker(t, client, orders)

	cancel, done := runWorker(t, w)
	defer cancel()

	require.Eventually(t, func() bool {
		return len(orders.snapshotUpdates()) == 2
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
	var listCalls atomic.Int32
	orders := &fakeOrders{
		listUnfinished: func(_ context.Context, _ int) ([]domain.Order, error) {
			n := listCalls.Add(1)
			if n > 1 {
				return nil, nil
			}
			return []domain.Order{{Number: "12345678903"}}, nil
		},
	}

	client := mocks.NewMockAccrualClient(t)
	client.EXPECT().
		GetOrder(mock.Anything, mock.Anything).
		Return(nil, &accrual.RateLimitError{RetryAfter: time.Hour}).
		Maybe()

	w := accrual.NewWorker(client, orders, accrual.WorkerConfig{
		PollInterval: 5 * time.Millisecond,
		Workers:      1,
		BatchLimit:   10,
	}, zap.NewNop())

	cancel, done := runWorker(t, w)
	defer cancel()
	time.Sleep(80 * time.Millisecond)
	cancel()
	<-done

	// Без rate-limit за 80мс при тике 5мс мы бы видели десятки вызовов ListUnfinished.
	// С rate-limit генератор должен «встать», и количество вызовов остаётся низким.
	assert.Less(t, listCalls.Load(), int32(5),
		"generator должен остановиться после rate-limit, вызовов: %d", listCalls.Load())
}
