// Package main — точка входа сервиса накопительной системы лояльности «Гофермарт».
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/warenikov/gofermart/internal/config"
	"github.com/warenikov/gofermart/internal/logger"
	"github.com/warenikov/gofermart/internal/repository"
	"github.com/warenikov/gofermart/internal/server"
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

	srv, err := server.New(server.Config{Addr: cfg.RunAddress}, logger.For(log, "server"))
	if err != nil {
		mainLog.Fatal("ошибка инициализации сервера", logger.Err(err))
	}

	if err := srv.Run(ctx); err != nil {
		mainLog.Fatal("ошибка работы сервера", logger.Err(err))
	}
}
