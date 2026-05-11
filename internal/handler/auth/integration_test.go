//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/auth"
	authh "github.com/warenikov/gofermart/internal/handler/auth"
	"github.com/warenikov/gofermart/internal/repository"
	authsvc "github.com/warenikov/gofermart/internal/service/auth"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("DATABASE_URI")
	if dsn == "" {
		log.Println("DATABASE_URI не задан — интеграционные тесты auth пропущены")
		os.Exit(0)
	}
	if err := repository.ApplyMigrations(dsn); err != nil {
		log.Fatalf("ошибка миграций: %v", err)
	}
	pool, err := repository.NewPool(context.Background(), dsn)
	if err != nil {
		log.Fatalf("ошибка подключения к БД: %v", err)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func resetDB(t *testing.T) {
	t.Helper()
	const q = `TRUNCATE ` +
		repository.TableWithdrawals + `, ` +
		repository.TableOrders + `, ` +
		repository.TableUsers +
		` RESTART IDENTITY CASCADE`
	_, err := testPool.Exec(context.Background(), q)
	require.NoError(t, err)
}

func startTestServer(t *testing.T) (url string, tokens *auth.TokenManager) {
	t.Helper()
	tokens, err := auth.NewTokenManager("integration-test-secret", time.Hour)
	require.NoError(t, err)

	userRepo := repository.NewUserRepository(testPool)
	svc := authsvc.NewService(userRepo, tokens, zap.NewNop())
	handler := authh.NewHandler(svc, zap.NewNop())

	r := chi.NewRouter()
	r.Post("/api/user/register", handler.Register)
	r.Post("/api/user/login", handler.Login)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts.URL, tokens
}

func postJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(),
		http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func bearerFromHeader(t *testing.T, resp *http.Response) string {
	t.Helper()
	h := resp.Header.Get("Authorization")
	require.NotEmpty(t, h, "Authorization header должен быть в ответе")
	require.True(t, strings.HasPrefix(h, "Bearer "), "ожидается префикс Bearer, получено: %q", h)
	return strings.TrimPrefix(h, "Bearer ")
}

func decodeToken(t *testing.T, resp *http.Response) string {
	t.Helper()
	var body struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NotEmpty(t, body.Token, "token в теле должен быть непустой")
	return body.Token
}

func TestAuth_Register_NewUser(t *testing.T) {
	resetDB(t)
	url, tokens := startTestServer(t)

	resp := postJSON(t, url+"/api/user/register",
		`{"login":"alice","password":"secret"}`)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	headerToken := bearerFromHeader(t, resp)
	bodyToken := decodeToken(t, resp)
	assert.Equal(t, headerToken, bodyToken)

	userID, err := tokens.Parse(headerToken)
	require.NoError(t, err)
	assert.NotZero(t, userID)
}

func TestAuth_Register_DuplicateLogin_Returns409(t *testing.T) {
	resetDB(t)
	url, _ := startTestServer(t)

	resp := postJSON(t, url+"/api/user/register",
		`{"login":"alice","password":"x"}`)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = postJSON(t, url+"/api/user/register",
		`{"login":"alice","password":"y"}`)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestAuth_Register_BadJSON_Returns400(t *testing.T) {
	resetDB(t)
	url, _ := startTestServer(t)

	resp := postJSON(t, url+"/api/user/register", `{not-json`)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAuth_Register_EmptyFields_Returns400(t *testing.T) {
	resetDB(t)
	url, _ := startTestServer(t)

	resp := postJSON(t, url+"/api/user/register",
		`{"login":"","password":""}`)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAuth_Login_ValidCredentials(t *testing.T) {
	resetDB(t)
	url, tokens := startTestServer(t)

	_ = postRegister(t, url, "alice", "secret")

	resp := postJSON(t, url+"/api/user/login",
		`{"login":"alice","password":"secret"}`)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	token := bearerFromHeader(t, resp)
	userID, err := tokens.Parse(token)
	require.NoError(t, err)
	assert.NotZero(t, userID)
}

func TestAuth_Login_WrongPassword_Returns401(t *testing.T) {
	resetDB(t)
	url, _ := startTestServer(t)

	_ = postRegister(t, url, "alice", "secret")

	resp := postJSON(t, url+"/api/user/login",
		`{"login":"alice","password":"wrong"}`)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAuth_Login_UnknownUser_Returns401(t *testing.T) {
	resetDB(t)
	url, _ := startTestServer(t)

	resp := postJSON(t, url+"/api/user/login",
		`{"login":"ghost","password":"x"}`)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAuth_Login_BadJSON_Returns400(t *testing.T) {
	resetDB(t)
	url, _ := startTestServer(t)

	resp := postJSON(t, url+"/api/user/login", `bad`)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func postRegister(t *testing.T, baseURL, login, password string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"login": login, "password": password})
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Context(),
		http.MethodPost, baseURL+"/api/user/register", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return bearerFromHeader(t, resp)
}
