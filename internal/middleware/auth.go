// Package middleware содержит HTTP-middleware сервиса.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/warenikov/gofermart/internal/auth"
	"github.com/warenikov/gofermart/internal/logger"
)

// TokenParser проверяет JWT и возвращает идентификатор пользователя.
type TokenParser interface {
	Parse(token string) (int64, error)
}

type userIDCtxKey struct{}

// UserIDFromCtx возвращает идентификатор аутентифицированного пользователя из контекста запроса.
// Возвращает false, если в контексте нет пользователя (запрос не прошёл [Auth]).
func UserIDFromCtx(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDCtxKey{}).(int64)
	return id, ok
}

// Auth — middleware, проверяющий заголовок Authorization: Bearer ...
// На успехе кладёт user_id в контекст запроса, на любой ошибке — 401.
func Auth(parser TokenParser, log *zap.Logger) func(http.Handler) http.Handler {
	mwLog := logger.For(log, "middleware.auth")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearerToken(r)
			if raw == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			userID, err := parser.Parse(raw)
			if err != nil {
				if errors.Is(err, auth.ErrInvalidToken) {
					mwLog.Debug("отклонён невалидный токен", logger.Err(err))
				} else {
					mwLog.Error("ошибка парсинга токена", logger.Err(err))
				}
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), userIDCtxKey{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
