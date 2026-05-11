package accrual

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/domain"
	"github.com/warenikov/gofermart/internal/logger"
)

// AccrualClient — поведение клиента к системе расчёта, нужное воркеру.
//
//nolint:revive // имя AccrualClient оправдано для ясности в чужом пакете
type AccrualClient interface {
	GetOrder(ctx context.Context, number string) (*OrderInfo, error)
}

// OrderUpdater — поведение репозитория заказов, нужное воркеру.
type OrderUpdater interface {
	ListUnfinished(ctx context.Context, limit int) ([]domain.Order, error)
	UpdateStatus(ctx context.Context, number string, status domain.OrderStatus, accrual decimal.NullDecimal) error
}

// WorkerConfig — параметры фонового поллера.
type WorkerConfig struct {
	// PollInterval — как часто опрашивать БД на наличие незавершённых заказов.
	PollInterval time.Duration
	// Workers — сколько параллельных goroutine-воркеров.
	Workers int
	// BatchLimit — сколько заказов за один тик брать из БД.
	BatchLimit int
}

const (
	defaultPollInterval = time.Second
	defaultWorkers      = 4
	defaultBatchLimit   = 100
)

func (c WorkerConfig) withDefaults() WorkerConfig {
	if c.PollInterval <= 0 {
		c.PollInterval = defaultPollInterval
	}
	if c.Workers <= 0 {
		c.Workers = defaultWorkers
	}
	if c.BatchLimit <= 0 {
		c.BatchLimit = defaultBatchLimit
	}
	return c
}

// Worker — фоновый поллер: периодически берёт незавершённые заказы из БД
// и обновляет их статусы по ответам системы accrual.
type Worker struct {
	client AccrualClient
	orders OrderUpdater
	cfg    WorkerConfig
	log    *zap.Logger
}

// NewWorker создаёт поллер. Нулевые значения cfg заменяются дефолтами.
func NewWorker(client AccrualClient, orders OrderUpdater, cfg WorkerConfig, log *zap.Logger) *Worker {
	return &Worker{
		client: client,
		orders: orders,
		cfg:    cfg.withDefaults(),
		log:    logger.For(log, "accrual.worker"),
	}
}

// Run запускает generator + N worker'ов и блокируется до отмены ctx.
// При отмене дренирует jobs и корректно завершает все goroutine'ы.
func (w *Worker) Run(ctx context.Context) error {
	jobs := make(chan string)

	var (
		rlMu        sync.Mutex
		rateLimited time.Time
	)
	setRateLimit := func(until time.Time) {
		rlMu.Lock()
		defer rlMu.Unlock()
		if until.After(rateLimited) {
			rateLimited = until
		}
	}
	rateLimitedUntil := func() time.Time {
		rlMu.Lock()
		defer rlMu.Unlock()
		return rateLimited
	}

	var wg sync.WaitGroup
	for range w.cfg.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for number := range jobs {
				w.processOne(ctx, number, setRateLimit)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(jobs)
		w.runGenerator(ctx, jobs, rateLimitedUntil)
	}()

	wg.Wait()
	w.log.Info("поллер accrual остановлен")
	return nil
}

func (w *Worker) runGenerator(ctx context.Context, jobs chan<- string, pausedUntil func() time.Time) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if until := pausedUntil(); time.Now().Before(until) {
			w.log.Debug("accrual на rate-limit, пропускаем тик", zap.Time("until", until))
			continue
		}

		list, err := w.orders.ListUnfinished(ctx, w.cfg.BatchLimit)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			w.log.Error("ошибка ListUnfinished", logger.Err(err))
			continue
		}

		for _, o := range list {
			select {
			case <-ctx.Done():
				return
			case jobs <- o.Number:
			}
		}
	}
}

func (w *Worker) processOne(ctx context.Context, number string, setRateLimit func(time.Time)) {
	rl, err := processOrder(ctx, w.client, w.orders, number)
	if rl != nil {
		w.log.Warn("accrual попросил подождать", zap.Duration("retry_after", rl.RetryAfter))
		setRateLimit(time.Now().Add(rl.RetryAfter))
		return
	}
	if err != nil && ctx.Err() == nil {
		w.log.Warn("ошибка обработки заказа", zap.String("number", number), logger.Err(err))
	}
}

// processOrder — чистая функция обработки одного заказа.
// Возвращает *RateLimitError, если accrual попросил подождать; nil-ошибка означает успех или пропуск (не зарегистрирован / unknown status).
func processOrder(
	ctx context.Context,
	client AccrualClient,
	orders OrderUpdater,
	number string,
) (*RateLimitError, error) {
	info, err := client.GetOrder(ctx, number)
	if err != nil {
		var rl *RateLimitError
		if errors.As(err, &rl) {
			return rl, nil
		}
		if errors.Is(err, ErrOrderNotRegistered) {
			return nil, nil
		}
		return nil, err
	}

	status, ok := mapStatus(info.Status)
	if !ok {
		return nil, nil
	}
	if err := orders.UpdateStatus(ctx, number, status, info.Accrual); err != nil {
		return nil, err
	}
	return nil, nil
}

func mapStatus(s Status) (domain.OrderStatus, bool) {
	switch s {
	case StatusRegistered, StatusProcessing:
		return domain.OrderStatusProcessing, true
	case StatusInvalid:
		return domain.OrderStatusInvalid, true
	case StatusProcessed:
		return domain.OrderStatusProcessed, true
	}
	return "", false
}
