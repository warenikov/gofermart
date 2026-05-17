//go:build integration

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// register регистрирует юзера и возвращает токен.
func register(t *testing.T, app *testApp, login, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"login": login, "password": password})
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost,
		app.server.URL+"/api/user/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	token := strings.TrimPrefix(resp.Header.Get("Authorization"), "Bearer ")
	require.NotEmpty(t, token)
	return token
}

func authedReq(t *testing.T, method, url, body, token string) *http.Request {
	t.Helper()
	var br io.Reader
	if body != "" {
		br = strings.NewReader(body)
	} else {
		br = http.NoBody
	}
	req, err := http.NewRequestWithContext(t.Context(), method, url, br)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// doStatus шлёт запрос, сбрасывает тело и возвращает только статус.
func doStatus(t *testing.T, req *http.Request) int {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func notFoundAccrual(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func TestOrders_Submit_Accepted_And_Duplicates(t *testing.T) {
	app := startTestApp(t, notFoundAccrual, false)
	token := register(t, app, "alice", "secret")

	// первая загрузка → 202
	status := doStatus(t, authedReq(t, http.MethodPost, app.server.URL+"/api/user/orders", "12345678903", token))
	require.Equal(t, http.StatusAccepted, status)

	// та же — 200 (уже загружен этим юзером)
	status = doStatus(t, authedReq(t, http.MethodPost, app.server.URL+"/api/user/orders", "12345678903", token))
	require.Equal(t, http.StatusOK, status)

	// другой пользователь — 409
	otherToken := register(t, app, "bob", "secret")
	status = doStatus(t, authedReq(t, http.MethodPost, app.server.URL+"/api/user/orders", "12345678903", otherToken))
	require.Equal(t, http.StatusConflict, status)
}

func TestOrders_Submit_InvalidLuhn_Returns422(t *testing.T) {
	app := startTestApp(t, notFoundAccrual, false)
	token := register(t, app, "alice", "secret")

	status := doStatus(t, authedReq(t, http.MethodPost, app.server.URL+"/api/user/orders", "12345678901", token))
	require.Equal(t, http.StatusUnprocessableEntity, status)
}

func TestOrders_Submit_NoAuth_Returns401(t *testing.T) {
	app := startTestApp(t, notFoundAccrual, false)

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost,
		app.server.URL+"/api/user/orders", strings.NewReader("12345678903"))
	status := doStatus(t, req)
	require.Equal(t, http.StatusUnauthorized, status)
}

func TestOrders_List_Empty_Returns204(t *testing.T) {
	app := startTestApp(t, notFoundAccrual, false)
	token := register(t, app, "alice", "secret")

	status := doStatus(t, authedReq(t, http.MethodGet, app.server.URL+"/api/user/orders", "", token))
	require.Equal(t, http.StatusNoContent, status)
}

func TestOrders_List_AfterSubmit_Returns200_WithNew(t *testing.T) {
	app := startTestApp(t, notFoundAccrual, false)
	token := register(t, app, "alice", "secret")

	doStatus(t, authedReq(t, http.MethodPost, app.server.URL+"/api/user/orders", "12345678903", token))

	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, app.server.URL+"/api/user/orders", "", token))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var orders []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&orders))
	require.Len(t, orders, 1)
	assert.Equal(t, "12345678903", orders[0]["number"])
	assert.Equal(t, "NEW", orders[0]["status"])
}

func TestWorker_UpdatesOrderToProcessed_AndBalance(t *testing.T) {
	app := startTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/12345678903") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"order":"12345678903","status":"PROCESSED","accrual":500}`))
	}, true)

	token := register(t, app, "alice", "secret")
	doStatus(t, authedReq(t, http.MethodPost, app.server.URL+"/api/user/orders", "12345678903", token))

	// ждём, пока поллер увидит accrual и обновит заказ
	require.Eventually(t, func() bool {
		resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, app.server.URL+"/api/user/balance", "", token))
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		var b map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&b)
		current, _ := b["current"].(float64)
		return current >= 500
	}, 3*time.Second, 50*time.Millisecond, "баланс должен дорасти до 500 после accrual")
}

func TestBalance_Withdraw_Success_And_InsufficientFunds(t *testing.T) {
	app := startTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/12345678903") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"order":"12345678903","status":"PROCESSED","accrual":1000}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}, true)

	token := register(t, app, "alice", "secret")
	doStatus(t, authedReq(t, http.MethodPost, app.server.URL+"/api/user/orders", "12345678903", token))

	// ждём пока баланс = 1000
	require.Eventually(t, func() bool {
		resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, app.server.URL+"/api/user/balance", "", token))
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		var b map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&b)
		current, _ := b["current"].(float64)
		return current >= 1000
	}, 3*time.Second, 50*time.Millisecond)

	// успешное списание
	status := doStatus(t, authedReq(t, http.MethodPost, app.server.URL+"/api/user/balance/withdraw",
		`{"order":"2377225624","sum":300}`, token))
	require.Equal(t, http.StatusOK, status)

	// баланс уменьшился
	resp2, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, app.server.URL+"/api/user/balance", "", token))
	require.NoError(t, err)
	var b map[string]any
	_ = json.NewDecoder(resp2.Body).Decode(&b)
	resp2.Body.Close()
	assert.InDelta(t, 700.0, b["current"], 0.001)
	assert.InDelta(t, 300.0, b["withdrawn"], 0.001)

	// списание сверх баланса → 402
	status = doStatus(t, authedReq(t, http.MethodPost, app.server.URL+"/api/user/balance/withdraw",
		`{"order":"346436439","sum":99999}`, token))
	require.Equal(t, http.StatusPaymentRequired, status)
}

func TestWithdrawals_List_Empty_Returns204(t *testing.T) {
	app := startTestApp(t, notFoundAccrual, false)
	token := register(t, app, "alice", "secret")

	status := doStatus(t, authedReq(t, http.MethodGet, app.server.URL+"/api/user/withdrawals", "", token))
	require.Equal(t, http.StatusNoContent, status)
}

func TestWithdrawals_List_AfterWithdraw_Returns200(t *testing.T) {
	app := startTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/12345678903") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"order":"12345678903","status":"PROCESSED","accrual":500}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}, true)

	token := register(t, app, "alice", "secret")
	doStatus(t, authedReq(t, http.MethodPost, app.server.URL+"/api/user/orders", "12345678903", token))

	require.Eventually(t, func() bool {
		resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, app.server.URL+"/api/user/balance", "", token))
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		var b map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&b)
		current, _ := b["current"].(float64)
		return current >= 500
	}, 3*time.Second, 50*time.Millisecond)

	status := doStatus(t, authedReq(t, http.MethodPost, app.server.URL+"/api/user/balance/withdraw",
		`{"order":"2377225624","sum":100}`, token))
	require.Equal(t, http.StatusOK, status)

	resp2, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, app.server.URL+"/api/user/withdrawals", "", token))
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&list))
	require.Len(t, list, 1)
	assert.Equal(t, "2377225624", list[0]["order"])
	assert.InDelta(t, 100.0, list[0]["sum"], 0.001)
	assert.NotEmpty(t, list[0]["processed_at"])
}
