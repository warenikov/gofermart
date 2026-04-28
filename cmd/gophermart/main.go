// Package main — точка входа сервиса накопительной системы лояльности «Гофермарт».
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/config"
	"github.com/warenikov/gofermart/internal/logger"
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

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Addr:         cfg.RunAddress,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		mainLog.Info("сервер запущен", zap.String("addr", cfg.RunAddress))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			mainLog.Fatal("ошибка запуска сервера", zap.Error(err))
		}
	}()

	<-ctx.Done()
	mainLog.Info("получен сигнал завершения, останавливаем сервер")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		mainLog.Error("ошибка graceful shutdown", zap.Error(err))
	}

	mainLog.Info("сервер остановлен")
}
