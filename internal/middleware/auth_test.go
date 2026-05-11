package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/auth"
	"github.com/warenikov/gofermart/internal/middleware"
)

type fakeParser struct {
	userID int64
	err    error
}

func (f *fakeParser) Parse(_ string) (int64, error) {
	return f.userID, f.err
}

func runMW(t *testing.T, parser middleware.TokenParser, req *http.Request) (status int, ctxUserID int64, hadUser bool) {
	t.Helper()
	rr := httptest.NewRecorder()
	handler := middleware.Auth(parser, zap.NewNop())(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctxUserID, hadUser = middleware.UserIDFromCtx(r.Context())
	}))
	handler.ServeHTTP(rr, req)
	return rr.Code, ctxUserID, hadUser
}

func TestAuth_MissingHeader_Returns401(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/protected", http.NoBody)
	status, _, hadUser := runMW(t, &fakeParser{userID: 42}, req)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.False(t, hadUser, "хендлер не должен вызываться")
}

func TestAuth_NoBearerPrefix_Returns401(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/protected", http.NoBody)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	status, _, hadUser := runMW(t, &fakeParser{userID: 42}, req)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.False(t, hadUser)
}

func TestAuth_InvalidToken_Returns401(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/protected", http.NoBody)
	req.Header.Set("Authorization", "Bearer bogus")
	status, _, hadUser := runMW(t, &fakeParser{err: auth.ErrInvalidToken}, req)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.False(t, hadUser)
}

func TestAuth_ValidToken_PassesUserID(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/protected", http.NoBody)
	req.Header.Set("Authorization", "Bearer valid-token")
	status, userID, hadUser := runMW(t, &fakeParser{userID: 123}, req)
	require.True(t, hadUser, "хендлер должен быть вызван")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, int64(123), userID)
}

func TestUserIDFromCtx_AbsentReturnsFalse(t *testing.T) {
	t.Parallel()
	id, ok := middleware.UserIDFromCtx(t.Context())
	assert.False(t, ok)
	assert.Zero(t, id)
}
