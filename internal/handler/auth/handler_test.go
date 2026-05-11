package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/domain"
	authh "github.com/warenikov/gofermart/internal/handler/auth"
	authsvc "github.com/warenikov/gofermart/internal/service/auth"
)

type fakeSvc struct {
	registerFn func(ctx context.Context, login, password string) (string, error)
	loginFn    func(ctx context.Context, login, password string) (string, error)
}

func (f *fakeSvc) Register(ctx context.Context, login, password string) (string, error) {
	return f.registerFn(ctx, login, password)
}

func (f *fakeSvc) Login(ctx context.Context, login, password string) (string, error) {
	return f.loginFn(ctx, login, password)
}

func TestHandler_Register_Success(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{registerFn: func(_ context.Context, login, password string) (string, error) {
		assert.Equal(t, "alice", login)
		assert.Equal(t, "secret", password)
		return "tok-1", nil
	}}
	h := authh.NewHandler(svc, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/user/register",
		strings.NewReader(`{"login":"alice","password":"secret"}`))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "Bearer tok-1", rr.Header().Get("Authorization"))
	assert.Contains(t, rr.Body.String(), `"token":"tok-1"`)
}

func TestHandler_Register_BadJSON_Returns400(t *testing.T) {
	t.Parallel()
	h := authh.NewHandler(&fakeSvc{}, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/user/register",
		strings.NewReader(`{not-json`))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandler_Register_MissingFields_Returns400(t *testing.T) {
	t.Parallel()
	h := authh.NewHandler(&fakeSvc{}, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/user/register",
		strings.NewReader(`{"login":"","password":""}`))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandler_Register_LoginTaken_Returns409(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{registerFn: func(_ context.Context, _, _ string) (string, error) {
		return "", domain.ErrLoginTaken
	}}
	h := authh.NewHandler(svc, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/user/register",
		strings.NewReader(`{"login":"alice","password":"x"}`))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
}

func TestHandler_Register_InvalidInput_Returns400(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{registerFn: func(_ context.Context, _, _ string) (string, error) {
		return "", authsvc.ErrInvalidInput
	}}
	h := authh.NewHandler(svc, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/user/register",
		strings.NewReader(`{"login":"a","password":"b"}`))
	rr := httptest.NewRecorder()
	h.Register(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandler_Register_Unknown_Returns500(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{registerFn: func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("boom")
	}}
	h := authh.NewHandler(svc, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/user/register",
		strings.NewReader(`{"login":"a","password":"b"}`))
	rr := httptest.NewRecorder()
	h.Register(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandler_Login_Success(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{loginFn: func(_ context.Context, login, password string) (string, error) {
		assert.Equal(t, "alice", login)
		assert.Equal(t, "secret", password)
		return "tok-2", nil
	}}
	h := authh.NewHandler(svc, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/user/login",
		strings.NewReader(`{"login":"alice","password":"secret"}`))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "Bearer tok-2", rr.Header().Get("Authorization"))
}

func TestHandler_Login_InvalidCreds_Returns401(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{loginFn: func(_ context.Context, _, _ string) (string, error) {
		return "", domain.ErrInvalidCredentials
	}}
	h := authh.NewHandler(svc, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/user/login",
		strings.NewReader(`{"login":"a","password":"b"}`))
	rr := httptest.NewRecorder()
	h.Login(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandler_Login_Unknown_Returns500(t *testing.T) {
	t.Parallel()
	svc := &fakeSvc{loginFn: func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("boom")
	}}
	h := authh.NewHandler(svc, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/user/login",
		strings.NewReader(`{"login":"a","password":"b"}`))
	rr := httptest.NewRecorder()
	h.Login(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandler_Login_BadJSON_Returns400(t *testing.T) {
	t.Parallel()
	h := authh.NewHandler(&fakeSvc{}, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(`bad`))
	rr := httptest.NewRecorder()
	h.Login(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
