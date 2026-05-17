//go:build integration

// Package e2e — end-to-end интеграционные тесты бизнес-логики через полный HTTP-стек.
package e2e

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/accrual"
	"github.com/warenikov/gofermart/internal/auth"
	authh "github.com/warenikov/gofermart/internal/handler/auth"
	balanceh "github.com/warenikov/gofermart/internal/handler/balance"
	orderh "github.com/warenikov/gofermart/internal/handler/order"
	mw "github.com/warenikov/gofermart/internal/middleware"
	"github.com/warenikov/gofermart/internal/repository"
	authsvc "github.com/warenikov/gofermart/internal/service/auth"
	balancesvc "github.com/warenikov/gofermart/internal/service/balance"
	ordersvc "github.com/warenikov/gofermart/internal/service/order"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(runE2E(m))
}

func runE2E(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pgC, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("gofermart_e2e"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		log.Printf("не удалось поднять postgres-контейнер: %v", err)
		return 1
	}
	defer func() {
		if err := testcontainers.TerminateContainer(pgC); err != nil {
			log.Printf("terminate postgres: %v", err)
		}
	}()

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Printf("DSN postgres: %v", err)
		return 1
	}

	pool, err := repository.NewPool(ctx, dsn)
	if err != nil {
		log.Printf("ошибка подключения к БД: %v", err)
		return 1
	}
	defer pool.Close()
	testPool = pool

	return m.Run()
}

func resetDB(t *testing.T) {
	t.Helper()
	const q = `TRUNCATE withdrawals, orders, users RESTART IDENTITY CASCADE`
	_, err := testPool.Exec(context.Background(), q)
	require.NoError(t, err)
}

// testApp — собранный набор зависимостей для одного теста.
type testApp struct {
	server  *httptest.Server
	tokens  *auth.TokenManager
	accrual *httptest.Server
	worker  *accrual.Worker
}

// startTestApp поднимает полный HTTP-сервер на httptest + worker + mock accrual.
// AccrualHandler — обработчик GET /api/orders/{number}, имитирующий accrual.
// Если poller=true, фоновый воркер запускается и работает до конца теста.
func startTestApp(t *testing.T, accrualHandler http.HandlerFunc, poller bool) *testApp {
	t.Helper()
	resetDB(t)

	mockAccrual := httptest.NewServer(accrualHandler)
	t.Cleanup(mockAccrual.Close)

	tokens, err := auth.NewTokenManager("e2e-test-secret", time.Hour)
	require.NoError(t, err)

	userRepo := repository.NewUserRepository(testPool)
	orderRepo := repository.NewOrderRepository(testPool)
	withdrawalRepo := repository.NewWithdrawalRepository(testPool)

	authService := authsvc.NewService(userRepo, tokens, zap.NewNop())
	orderService := ordersvc.NewService(orderRepo, zap.NewNop())
	balanceService := balancesvc.NewService(withdrawalRepo, zap.NewNop())

	authHandler := authh.NewHandler(authService, zap.NewNop())
	orderHandler := orderh.NewHandler(orderService, zap.NewNop())
	balanceHandler := balanceh.NewHandler(balanceService, zap.NewNop())

	client := accrual.New(mockAccrual.URL, time.Second, nil)
	worker := accrual.NewWorker(client, orderRepo, accrual.WorkerConfig{
		PollInterval: 20 * time.Millisecond,
		Workers:      2,
		BatchLimit:   100,
	}, zap.NewNop())

	r := chi.NewRouter()
	r.Route("/api/user", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Group(func(r chi.Router) {
			r.Use(mw.Auth(tokens, zap.NewNop()))
			r.Post("/orders", orderHandler.Submit)
			r.Get("/orders", orderHandler.List)
			r.Get("/balance", balanceHandler.Get)
			r.Post("/balance/withdraw", balanceHandler.Withdraw)
			r.Get("/withdrawals", balanceHandler.ListWithdrawals)
		})
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	app := &testApp{server: srv, tokens: tokens, accrual: mockAccrual, worker: worker}

	if poller {
		workerCtx, cancelWorker := context.WithCancel(context.Background())
		t.Cleanup(cancelWorker)

		workerDone := make(chan struct{})
		go func() {
			defer close(workerDone)
			_ = worker.Run(workerCtx)
		}()
		t.Cleanup(func() {
			cancelWorker()
			select {
			case <-workerDone:
			case <-time.After(2 * time.Second):
				t.Error("worker не завершился за 2s")
			}
		})
	}

	return app
}
