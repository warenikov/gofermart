// Package accrual — клиент к внешней системе расчёта начислений.
package accrual

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/samber/oops"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/logger"
)

// ErrOrderNotRegistered — accrual вернул 204: заказ не зарегистрирован в системе расчёта.
var ErrOrderNotRegistered = errors.New("order not registered in accrual")

// RateLimitError — accrual вернул 429 и просит подождать RetryAfter перед следующим запросом.
type RateLimitError struct {
	RetryAfter time.Duration
}

// Error реализует interface error.
func (e *RateLimitError) Error() string {
	return fmt.Sprintf("accrual rate limited; retry after %s", e.RetryAfter)
}

// Status — статус расчёта на стороне accrual.
type Status string

// Статусы из спеки accrual.
const (
	StatusRegistered Status = "REGISTERED"
	StatusProcessing Status = "PROCESSING"
	StatusInvalid    Status = "INVALID"
	StatusProcessed  Status = "PROCESSED"
)

// OrderInfo — ответ accrual для одного заказа.
type OrderInfo struct {
	Order   string
	Status  Status
	Accrual decimal.NullDecimal
}

// Client — HTTP-клиент к системе расчёта начислений.
// Использует retryablehttp для ретраев на 5xx/сетевых ошибках; 429 НЕ ретраит —
// его выдаёт наружу как [RateLimitError], чтобы воркер мог реализовать backoff.
type Client struct {
	baseURL string
	http    *http.Client
	log     *zap.Logger
}

const (
	defaultTimeout   = 5 * time.Second
	defaultRetryMax  = 3
	retryWaitMinimum = 200 * time.Millisecond
	retryWaitMaximum = 2 * time.Second
)

// New создаёт клиент с указанным baseURL и таймаутом запроса.
// Если timeout == 0, используется 5 секунд.
// Логгер используется для диагностики ретраев и невалидного Retry-After; nil — без логирования.
func New(baseURL string, timeout time.Duration, log *zap.Logger) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if log == nil {
		log = zap.NewNop()
	}
	clog := logger.For(log, "accrual.client")

	rc := retryablehttp.NewClient()
	rc.RetryMax = defaultRetryMax
	rc.RetryWaitMin = retryWaitMinimum
	rc.RetryWaitMax = retryWaitMaximum
	rc.Logger = nil // не льём в stderr
	rc.HTTPClient.Timeout = timeout
	rc.CheckRetry = retryPolicyKeep429

	return &Client{
		baseURL: baseURL,
		http:    rc.StandardClient(),
		log:     clog,
	}
}

// retryPolicyKeep429 — стандартная политика ретраев, но 429 пропускается
// без ретраев (его обрабатывает вызывающий код через [RateLimitError]).
func retryPolicyKeep429(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		return false, nil
	}
	return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
}

// GetOrder запрашивает информацию о начислении для заказа.
// Возможные ошибки:
//   - ErrOrderNotRegistered — accrual вернул 204
//   - *RateLimitError — accrual вернул 429
//   - oops-ошибка для всего остального (сеть, 5xx после исчерпания ретраев, невалидный JSON)
func (c *Client) GetOrder(ctx context.Context, number string) (*OrderInfo, error) {
	url := c.baseURL + "/api/orders/" + number
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, oops.In("accrual.client").Code("build_request").
			Wrapf(err, "сформировать запрос")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, oops.In("accrual.client").Code("do").With("number", number).
			Wrapf(err, "выполнить запрос к accrual")
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		return decodeOK(resp.Body)

	case http.StatusNoContent:
		return nil, ErrOrderNotRegistered

	case http.StatusTooManyRequests:
		return nil, &RateLimitError{RetryAfter: c.parseRetryAfter(resp.Header.Get("Retry-After"))}

	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, oops.In("accrual.client").Code("unexpected_status").
			With("status", resp.StatusCode).
			With("body", string(body)).
			Errorf("неожиданный статус от accrual: %d", resp.StatusCode)
	}
}

type orderDTO struct {
	Order   string           `json:"order"`
	Status  Status           `json:"status"`
	Accrual *decimal.Decimal `json:"accrual,omitempty"`
}

func decodeOK(body io.Reader) (*OrderInfo, error) {
	var dto orderDTO
	if err := json.NewDecoder(body).Decode(&dto); err != nil {
		return nil, oops.In("accrual.client").Code("decode").
			Wrapf(err, "разобрать ответ accrual")
	}
	info := &OrderInfo{Order: dto.Order, Status: dto.Status}
	if dto.Accrual != nil {
		info.Accrual = decimal.NullDecimal{Decimal: *dto.Accrual, Valid: true}
	}
	return info, nil
}

const defaultRetryAfter = 60 * time.Second

func (c *Client) parseRetryAfter(h string) time.Duration {
	if h == "" {
		return defaultRetryAfter
	}
	if secs, err := strconv.Atoi(h); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	c.log.Warn("не удалось разобрать Retry-After, используем дефолт",
		zap.String("header", h),
		zap.Duration("default", defaultRetryAfter))
	return defaultRetryAfter
}
