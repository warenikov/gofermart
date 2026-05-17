package order_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/domain"
	orderh "github.com/warenikov/gofermart/internal/handler/order"
	mw "github.com/warenikov/gofermart/internal/middleware"
)

type fakeSvc struct {
	submitFn func(ctx context.Context, userID int64, number string) error
	listFn   func(ctx context.Context, userID int64) ([]domain.Order, error)
}

func (f *fakeSvc) Submit(ctx context.Context, userID int64, number string) error {
	return f.submitFn(ctx, userID, number)
}

func (f *fakeSvc) List(ctx context.Context, userID int64) ([]domain.Order, error) {
	return f.listFn(ctx, userID)
}

// requestWithUser строит запрос с заранее установленным user_id в контексте,
// эмулируя поведение middleware.Auth.
func requestWithUser(t *testing.T, method, target string, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer x")
	parser := &fixedParser{userID: 42}

	var captured *http.Request
	mw.Auth(parser, zap.NewNop())(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r
	})).ServeHTTP(httptest.NewRecorder(), req)
	require.NotNil(t, captured, "middleware должен был передать запрос дальше")
	return captured
}

type fixedParser struct{ userID int64 }

func (p *fixedParser) Parse(_ string) (int64, error) { return p.userID, nil }

func TestHandler_Submit_Accepted(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{submitFn: func(_ context.Context, userID int64, number string) error {
		assert.Equal(t, int64(42), userID)
		assert.Equal(t, "12345678903", number)
		return nil
	}}
	h := orderh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodPost, "/api/user/orders", "12345678903\n")
	rr := httptest.NewRecorder()
	h.Submit(rr, req)
	assert.Equal(t, http.StatusAccepted, rr.Code)
}

func TestHandler_Submit_AlreadyOwned_Returns200(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{submitFn: func(_ context.Context, _ int64, _ string) error {
		return domain.ErrOrderAlreadyOwned
	}}
	h := orderh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodPost, "/api/user/orders", "12345678903")
	rr := httptest.NewRecorder()
	h.Submit(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandler_Submit_OwnedByOther_Returns409(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{submitFn: func(_ context.Context, _ int64, _ string) error {
		return domain.ErrOrderOwnedByOther
	}}
	h := orderh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodPost, "/api/user/orders", "12345678903")
	rr := httptest.NewRecorder()
	h.Submit(rr, req)
	assert.Equal(t, http.StatusConflict, rr.Code)
}

func TestHandler_Submit_InvalidLuhn_Returns422(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{submitFn: func(_ context.Context, _ int64, _ string) error {
		return domain.ErrInvalidLuhn
	}}
	h := orderh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodPost, "/api/user/orders", "12345678901")
	rr := httptest.NewRecorder()
	h.Submit(rr, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
}

func TestHandler_Submit_EmptyBody_Returns400(t *testing.T) {
	t.Parallel()
	h := orderh.NewHandler(&fakeSvc{}, zap.NewNop())

	req := requestWithUser(t, http.MethodPost, "/api/user/orders", "")
	rr := httptest.NewRecorder()
	h.Submit(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandler_Submit_UnknownError_Returns500(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{submitFn: func(_ context.Context, _ int64, _ string) error {
		return errors.New("boom")
	}}
	h := orderh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodPost, "/api/user/orders", "12345678903")
	rr := httptest.NewRecorder()
	h.Submit(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandler_Submit_NoUserCtx_Returns401(t *testing.T) {
	t.Parallel()
	h := orderh.NewHandler(&fakeSvc{}, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/user/orders", strings.NewReader("12345678903"))
	rr := httptest.NewRecorder()
	h.Submit(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandler_List_200_WithAccrual(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{listFn: func(_ context.Context, _ int64) ([]domain.Order, error) {
		return []domain.Order{
			{
				Number: "12345678903",
				Status: domain.OrderStatusProcessed,
				Accrual: decimal.NullDecimal{
					Decimal: decimal.NewFromFloat(500.5),
					Valid:   true,
				},
			},
			{
				Number: "346436439",
				Status: domain.OrderStatusInvalid,
			},
		}, nil
	}}
	h := orderh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodGet, "/api/user/orders", "")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var out []map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Len(t, out, 2)
	assert.Equal(t, "12345678903", out[0]["number"])
	assert.Equal(t, "PROCESSED", out[0]["status"])
	assert.InDelta(t, 500.5, out[0]["accrual"], 0.0001)
	_, hasAccrual := out[1]["accrual"]
	assert.False(t, hasAccrual, "accrual должен отсутствовать когда Valid=false")
}

func TestHandler_List_204_WhenEmpty(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{listFn: func(_ context.Context, _ int64) ([]domain.Order, error) {
		return nil, nil
	}}
	h := orderh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodGet, "/api/user/orders", "")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestHandler_List_500_OnError(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{listFn: func(_ context.Context, _ int64) ([]domain.Order, error) {
		return nil, errors.New("boom")
	}}
	h := orderh.NewHandler(svc, zap.NewNop())

	req := requestWithUser(t, http.MethodGet, "/api/user/orders", "")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}
