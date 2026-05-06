// Package server инкапсулирует HTTP-сервер сервиса: сборку роутера, запуск и graceful shutdown.
package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/samber/oops"
	"go.uber.org/zap"
)

// Config — параметры HTTP-сервера. Нулевые значения заменяются дефолтами в [New].
type Config struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

const (
	defaultReadTimeout     = 10 * time.Second
	defaultWriteTimeout    = 10 * time.Second
	defaultIdleTimeout     = 60 * time.Second
	defaultShutdownTimeout = 5 * time.Second
)

// Server — обёртка над *http.Server со встроенным логгером и таймаутом graceful shutdown.
type Server struct {
	httpSrv         *http.Server
	log             *zap.Logger
	shutdownTimeout time.Duration
}

// New собирает HTTP-сервер с маршрутами и middleware.
// Адрес обязателен; остальные таймауты — дефолтные, если не заданы.
func New(cfg Config, log *zap.Logger) (*Server, error) {
	if cfg.Addr == "" {
		return nil, oops.In("server").Code("invalid_config").Errorf("адрес HTTP-сервера не задан")
	}

	cfg = withDefaults(cfg)

	return &Server{
		httpSrv: &http.Server{
			Addr:         cfg.Addr,
			Handler:      newRouter(),
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
		log:             log,
		shutdownTimeout: cfg.ShutdownTimeout,
	}, nil
}

// Run запускает сервер и блокируется до отмены ctx или фатальной ошибки слушателя.
// При отмене ctx выполняется graceful shutdown с таймаутом из конфига.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("сервер запущен", zap.String("addr", s.httpSrv.Addr))
		err := s.httpSrv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- oops.In("server").Code("listen").Wrapf(err, "запуск HTTP-сервера")
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		return s.shutdown()
	case err := <-errCh:
		return err
	}
}

func (s *Server) shutdown() error {
	s.log.Info("получен сигнал завершения, останавливаем сервер")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
		return oops.In("server").Code("shutdown").Wrapf(err, "graceful shutdown HTTP-сервера")
	}
	s.log.Info("сервер остановлен")
	return nil
}

func withDefaults(cfg Config) Config {
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = defaultReadTimeout
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = defaultWriteTimeout
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = defaultIdleTimeout
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}
	return cfg
}
