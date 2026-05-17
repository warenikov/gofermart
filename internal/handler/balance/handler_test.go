package balance_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/domain"
	balanceh "github.com/warenikov/gofermart/internal/handler/balance"
	mw "github.com/warenikov/gofermart/internal/middleware"
	balancesvc "github.com/warenikov/gofermart/internal/service/balance"
)

type fakeSvc struct {
	getFn      func(ctx context.Context, userID int64) (domain.Balance, error)
	withdrawFn func(ctx context.Context, userID int64, order string, sum decimal.Decimal) error
	listFn     func(ctx context.Context, userID int64) ([]domain.Withdrawal, error)
}

func (f *fakeSvc) Get(ctx context.Context, userID int64) (domain.Balance, error) {
	return f.getFn(ctx, userID)
}

func (f *fakeSvc) Withdraw(ctx context.Context, userID int64, order string, sum decimal.Decimal) error {
	return f.withdrawFn(ctx, userID, order, sum)
}

func (f *fakeSvc) ListWithdrawals(ctx context.Context, userID int64) ([]domain.Withdrawal, error) {
	return f.listFn(ctx, userID)
}

type fixedParser struct{ userID int64 }

func (p *fixedParser) Parse(_ string) (int64, error) { return p.userID, nil }

func requestWithUser(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer x")
	var captured *http.Request
	mw.Auth(&fixedParser{userID: 42}, zap.NewNop())(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r
	})).ServeHTTP(httptest.NewRecorder(), req)
	require.NotNil(t, captured)
	return captured
}

func TestHandler_Get_Returns200(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{getFn: func(_ context.Context, _ int64) (domain.Balance, error) {
		return domain.Balance{
			Current:   decimal.NewFromFloat(500.5),
			Withdrawn: decimal.NewFromInt(42),
		}, nil
	}}
	h := balanceh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodGet, "/api/user/balance", "")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.InDelta(t, 500.5, got["current"], 0.0001)
	assert.InDelta(t, 42.0, got["withdrawn"], 0.0001)
}

func TestHandler_Get_NoUserCtx_Returns401(t *testing.T) {
	t.Parallel()
	h := balanceh.NewHandler(&fakeSvc{}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/user/balance", http.NoBody)
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandler_Get_500_OnError(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{getFn: func(_ context.Context, _ int64) (domain.Balance, error) {
		return domain.Balance{}, errors.New("boom")
	}}
	h := balanceh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodGet, "/api/user/balance", "")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandler_Withdraw_OK(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{withdrawFn: func(_ context.Context, userID int64, order string, sum decimal.Decimal) error {
		assert.Equal(t, int64(42), userID)
		assert.Equal(t, "2377225624", order)
		assert.True(t, sum.Equal(decimal.NewFromInt(751)))
		return nil
	}}
	h := balanceh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodPost, "/api/user/balance/withdraw",
		`{"order":"2377225624","sum":751}`)
	rr := httptest.NewRecorder()
	h.Withdraw(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandler_Withdraw_InsufficientFunds_Returns402(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{withdrawFn: func(_ context.Context, _ int64, _ string, _ decimal.Decimal) error {
		return domain.ErrInsufficientFunds
	}}
	h := balanceh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodPost, "/api/user/balance/withdraw",
		`{"order":"2377225624","sum":100}`)
	rr := httptest.NewRecorder()
	h.Withdraw(rr, req)
	assert.Equal(t, http.StatusPaymentRequired, rr.Code)
}

func TestHandler_Withdraw_InvalidLuhn_Returns422(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{withdrawFn: func(_ context.Context, _ int64, _ string, _ decimal.Decimal) error {
		return domain.ErrInvalidLuhn
	}}
	h := balanceh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodPost, "/api/user/balance/withdraw",
		`{"order":"23772","sum":100}`)
	rr := httptest.NewRecorder()
	h.Withdraw(rr, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
}

func TestHandler_Withdraw_InvalidSum_Returns400(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{withdrawFn: func(_ context.Context, _ int64, _ string, _ decimal.Decimal) error {
		return balancesvc.ErrInvalidSum
	}}
	h := balanceh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodPost, "/api/user/balance/withdraw",
		`{"order":"2377225624","sum":0}`)
	rr := httptest.NewRecorder()
	h.Withdraw(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandler_Withdraw_BadJSON_Returns400(t *testing.T) {
	t.Parallel()
	h := balanceh.NewHandler(&fakeSvc{}, zap.NewNop())

	req := requestWithUser(t, http.MethodPost, "/api/user/balance/withdraw", `{bad json`)
	rr := httptest.NewRecorder()
	h.Withdraw(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandler_Withdraw_MissingOrder_Returns400(t *testing.T) {
	t.Parallel()
	h := balanceh.NewHandler(&fakeSvc{}, zap.NewNop())

	req := requestWithUser(t, http.MethodPost, "/api/user/balance/withdraw",
		`{"order":"","sum":100}`)
	rr := httptest.NewRecorder()
	h.Withdraw(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandler_ListWithdrawals_200(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{listFn: func(_ context.Context, _ int64) ([]domain.Withdrawal, error) {
		return []domain.Withdrawal{
			{OrderNumber: "2377225624", Sum: decimal.NewFromInt(500), ProcessedAt: time.Now()},
		}, nil
	}}
	h := balanceh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodGet, "/api/user/withdrawals", "")
	rr := httptest.NewRecorder()
	h.ListWithdrawals(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var out []map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Len(t, out, 1)
	assert.Equal(t, "2377225624", out[0]["order"])
	assert.InDelta(t, 500.0, out[0]["sum"], 0.0001)
	assert.NotEmpty(t, out[0]["processed_at"])
}

func TestHandler_ListWithdrawals_204_WhenEmpty(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{listFn: func(_ context.Context, _ int64) ([]domain.Withdrawal, error) {
		return nil, nil
	}}
	h := balanceh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodGet, "/api/user/withdrawals", "")
	rr := httptest.NewRecorder()
	h.ListWithdrawals(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}
