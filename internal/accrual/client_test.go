package accrual_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/warenikov/gofermart/internal/accrual"
)

func TestClient_GetOrder_OK_WithAccrual(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/orders/12345678903", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"order":"12345678903","status":"PROCESSED","accrual":500.5}`))
	}))
	defer srv.Close()

	cl := accrual.New(srv.URL, time.Second)
	info, err := cl.GetOrder(t.Context(), "12345678903")
	require.NoError(t, err)
	assert.Equal(t, "12345678903", info.Order)
	assert.Equal(t, accrual.StatusProcessed, info.Status)
	require.True(t, info.Accrual.Valid)
	assert.True(t, info.Accrual.Decimal.Equal(decimal.NewFromFloat(500.5)))
}

func TestClient_GetOrder_OK_WithoutAccrual(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"order":"12345678903","status":"PROCESSING"}`))
	}))
	defer srv.Close()

	cl := accrual.New(srv.URL, time.Second)
	info, err := cl.GetOrder(t.Context(), "12345678903")
	require.NoError(t, err)
	assert.Equal(t, accrual.StatusProcessing, info.Status)
	assert.False(t, info.Accrual.Valid)
}

func TestClient_GetOrder_204_NotRegistered(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cl := accrual.New(srv.URL, time.Second)
	_, err := cl.GetOrder(t.Context(), "12345678903")
	require.Error(t, err)
	assert.True(t, errors.Is(err, accrual.ErrOrderNotRegistered))
}

func TestClient_GetOrder_429_RetryAfterSeconds(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	cl := accrual.New(srv.URL, time.Second)
	_, err := cl.GetOrder(t.Context(), "12345678903")
	require.Error(t, err)
	var rl *accrual.RateLimitError
	require.True(t, errors.As(err, &rl), "ожидается *RateLimitError, получено: %v", err)
	assert.Equal(t, 42*time.Second, rl.RetryAfter)
}

func TestClient_GetOrder_429_NoRetryAfter_FallsBackToDefault(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	cl := accrual.New(srv.URL, time.Second)
	_, err := cl.GetOrder(t.Context(), "12345678903")
	require.Error(t, err)
	var rl *accrual.RateLimitError
	require.True(t, errors.As(err, &rl))
	assert.Equal(t, 60*time.Second, rl.RetryAfter)
}

func TestClient_GetOrder_500_ReturnsOopsError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cl := accrual.New(srv.URL, time.Second)
	_, err := cl.GetOrder(t.Context(), "12345678903")
	require.Error(t, err)
	assert.False(t, errors.Is(err, accrual.ErrOrderNotRegistered))
	var rl *accrual.RateLimitError
	assert.False(t, errors.As(err, &rl))
}

func TestClient_GetOrder_BadJSON_ReturnsOopsError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not-json`))
	}))
	defer srv.Close()

	cl := accrual.New(srv.URL, time.Second)
	_, err := cl.GetOrder(t.Context(), "12345678903")
	require.Error(t, err)
}
