package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestNew_EmptyAddr_ReturnsOopsError(t *testing.T) {
	t.Parallel()

	_, err := New(Config{Addr: ""}, zap.NewNop())
	require.Error(t, err)

	var oerr oops.OopsError
	require.True(t, errors.As(err, &oerr), "ожидается обёрнутая в oops ошибка, получено: %v", err)
	assert.Equal(t, "server", oerr.Domain())
	assert.Equal(t, "invalid_config", oerr.Code())
}

func TestRouter_Health_Returns200(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := startServer(t, ctx, addr)
	waitReady(t, addr)

	resp, err := http.Get("http://" + addr + "/health") //nolint:noctx
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	cancel()
	require.NoError(t, <-srv)
}

func TestRun_CtxCancel_ShutsDownGracefully(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := startServer(t, ctx, addr)
	waitReady(t, addr)

	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершился после отмены контекста")
	}
}

func TestRun_BusyPort_ReturnsListenError(t *testing.T) {
	t.Parallel()

	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	addr := ln.Addr().String()

	srv, err := New(Config{Addr: addr}, zap.NewNop())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := srv.Run(ctx)
	require.Error(t, runErr)

	var oerr oops.OopsError
	require.True(t, errors.As(runErr, &oerr), "ожидается oops-ошибка, получено: %v", runErr)
	assert.Equal(t, "server", oerr.Domain())
	assert.Equal(t, "listen", oerr.Code())
}

// startServer запускает Server в goroutine и возвращает канал с результатом Run.
func startServer(t *testing.T, ctx context.Context, addr string) <-chan error {
	t.Helper()

	srv, err := New(Config{Addr: addr, ShutdownTimeout: time.Second}, zap.NewNop())
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()
	return errCh
}

// freeAddr возвращает свободный TCP-адрес. Возможна редкая гонка: между Close и
// последующим Listen порт может занять другой процесс. Для теста допустимо.
func freeAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// waitReady пингует /health, пока сервер не начнёт принимать соединения.
func waitReady(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/health") //nolint:noctx
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("сервер не стал готов на %s за 2с", addr)
}
