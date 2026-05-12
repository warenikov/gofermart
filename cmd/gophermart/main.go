// Package main — точка входа сервиса накопительной системы лояльности «Гофермарт».
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/warenikov/gofermart/internal/accrual"
	"github.com/warenikov/gofermart/internal/auth"
	"github.com/warenikov/gofermart/internal/config"
	authh "github.com/warenikov/gofermart/internal/handler/auth"
	balanceh "github.com/warenikov/gofermart/internal/handler/balance"
	orderh "github.com/warenikov/gofermart/internal/handler/order"
	"github.com/warenikov/gofermart/internal/logger"
	"github.com/warenikov/gofermart/internal/repository"
	"github.com/warenikov/gofermart/internal/server"
	authsvc "github.com/warenikov/gofermart/internal/service/auth"
	balancesvc "github.com/warenikov/gofermart/internal/service/balance"
	ordersvc "github.com/warenikov/gofermart/internal/service/order"
)

func main() {
	cfg, err := config.New()
	if err != nil {
		panic(err)
	}

	log, err := logger.New(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		panic(err)
	}
	defer log.Sync() //nolint:errcheck

	mainLog := logger.For(log, "main")

	if cfg.DatabaseURI == "" {
		mainLog.Fatal("DATABASE_URI не задан, сервис не может запуститься")
	}
	if cfg.JWTSecret == "" {
		mainLog.Fatal("JWT_SECRET не задан, сервис не может запуститься")
	}
	if cfg.AccrualSystemAddress == "" {
		mainLog.Fatal("ACCRUAL_SYSTEM_ADDRESS не задан, сервис не может запуститься")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := repository.ApplyMigrations(cfg.DatabaseURI); err != nil {
		mainLog.Fatal("ошибка применения миграций", logger.Err(err))
	}
	mainLog.Info("миграции применены")

	pool, err := repository.NewPool(ctx, cfg.DatabaseURI)
	if err != nil {
		mainLog.Fatal("ошибка подключения к БД", logger.Err(err))
	}
	defer pool.Close()
	mainLog.Info("подключение к БД установлено")

	tokens, err := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL)
	if err != nil {
		mainLog.Fatal("ошибка инициализации токен-менеджера", logger.Err(err))
	}

	userRepo := repository.NewUserRepository(pool)
	orderRepo := repository.NewOrderRepository(pool)
	withdrawalRepo := repository.NewWithdrawalRepository(pool)

	authService := authsvc.NewService(userRepo, tokens, log)
	orderService := ordersvc.NewService(orderRepo, log)
	balanceService := balancesvc.NewService(withdrawalRepo, log)

	authHandler := authh.NewHandler(authService, log)
	orderHandler := orderh.NewHandler(orderService, log)
	balanceHandler := balanceh.NewHandler(balanceService, log)

	accrualClient := accrual.New(cfg.AccrualSystemAddress, 0)
	worker := accrual.NewWorker(accrualClient, orderRepo, accrual.WorkerConfig{}, log)

	srv, err := server.New(
		server.Config{Addr: cfg.RunAddress},
		server.Deps{
			Auth:        authHandler,
			Order:       orderHandler,
			Balance:     balanceHandler,
			TokenParser: tokens,
		},
		logger.For(log, "server"),
	)
	if err != nil {
		mainLog.Fatal("ошибка инициализации сервера", logger.Err(err))
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return srv.Run(gCtx) })
	g.Go(func() error { return worker.Run(gCtx) })

	mainLog.Info("сервис запущен", zap.String("addr", cfg.RunAddress))

	if err := g.Wait(); err != nil {
		mainLog.Fatal("сервис завершился с ошибкой", logger.Err(err))
	}
	mainLog.Info("сервис остановлен")
}
