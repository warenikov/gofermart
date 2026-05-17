package auth_test

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/warenikov/gofermart/internal/auth"
)

const testSecret = "test-secret-do-not-use-in-prod"

func TestNewTokenManager_RejectsEmptySecret(t *testing.T) {
	t.Parallel()
	_, err := auth.NewTokenManager("", time.Hour)
	require.Error(t, err)
}

func TestNewTokenManager_RejectsNonPositiveTTL(t *testing.T) {
	t.Parallel()
	_, err := auth.NewTokenManager(testSecret, 0)
	require.Error(t, err)
	_, err = auth.NewTokenManager(testSecret, -time.Second)
	require.Error(t, err)
}

func TestTokenManager_IssueParse_RoundTrip(t *testing.T) {
	t.Parallel()
	tm, err := auth.NewTokenManager(testSecret, time.Hour)
	require.NoError(t, err)

	token, err := tm.Issue(42)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	userID, err := tm.Parse(token)
	require.NoError(t, err)
	assert.Equal(t, int64(42), userID)
}

func TestTokenManager_Parse_RejectsExpiredToken(t *testing.T) {
	t.Parallel()
	tm, err := auth.NewTokenManager(testSecret, time.Millisecond)
	require.NoError(t, err)

	token, err := tm.Issue(7)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	_, err = tm.Parse(token)
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidToken), "ожидается ErrInvalidToken, получено: %v", err)
}

func TestTokenManager_Parse_RejectsWrongSecret(t *testing.T) {
	t.Parallel()
	issuer, _ := auth.NewTokenManager(testSecret, time.Hour)
	verifier, _ := auth.NewTokenManager("other-secret", time.Hour)

	token, err := issuer.Issue(1)
	require.NoError(t, err)

	_, err = verifier.Parse(token)
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidToken))
}

func TestTokenManager_Parse_RejectsGarbage(t *testing.T) {
	t.Parallel()
	tm, _ := auth.NewTokenManager(testSecret, time.Hour)

	cases := []string{"", "not-a-jwt", "aaa.bbb.ccc"}
	for _, raw := range cases {
		_, err := tm.Parse(raw)
		require.Error(t, err, "должна быть ошибка для %q", raw)
		assert.True(t, errors.Is(err, auth.ErrInvalidToken), "raw=%q err=%v", raw, err)
	}
}

func TestTokenManager_Parse_RejectsAlgNone(t *testing.T) {
	t.Parallel()
	tm, _ := auth.NewTokenManager(testSecret, time.Hour)

	// Конструируем токен с alg=none — он не должен пройти проверку.
	claims := auth.Claims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	raw, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = tm.Parse(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidToken))
}
