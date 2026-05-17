// Package auth содержит примитивы аутентификации: выпуск и проверку JWT-токенов.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/samber/oops"
)

// ErrInvalidToken — токен отсутствует, испорчен, истёк или подписан не нашим ключом.
var ErrInvalidToken = errors.New("invalid token")

// Claims — полезная нагрузка JWT с идентификатором пользователя.
type Claims struct {
	UserID int64 `json:"uid"`
	jwt.RegisteredClaims
}

// TokenManager выпускает и проверяет JWT-токены на HS256.
type TokenManager struct {
	secret []byte
	ttl    time.Duration
}

// NewTokenManager создаёт менеджер токенов.
// Секрет не должен быть пустым, TTL — положительным.
func NewTokenManager(secret string, ttl time.Duration) (*TokenManager, error) {
	if secret == "" {
		return nil, oops.In("auth").Code("invalid_config").Errorf("JWT secret не задан")
	}
	if ttl <= 0 {
		return nil, oops.In("auth").Code("invalid_config").Errorf("JWT TTL должен быть > 0, получено %v", ttl)
	}
	return &TokenManager{secret: []byte(secret), ttl: ttl}, nil
}

// Issue выпускает токен для указанного пользователя.
func (m *TokenManager) Issue(userID int64) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", oops.In("auth").Code("sign").With("user_id", userID).
			Wrapf(err, "подписать токен")
	}
	return signed, nil
}

// Parse проверяет токен и возвращает идентификатор пользователя.
// Любая ошибка валидации (плохая подпись, истёкший токен, подделка) возвращает ErrInvalidToken.
func (m *TokenManager) Parse(raw string) (int64, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("неожиданный алгоритм подписи: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil || token == nil || !token.Valid {
		return 0, oops.In("auth").Code("parse").Wrap(ErrInvalidToken)
	}
	if claims.UserID == 0 {
		return 0, oops.In("auth").Code("missing_uid").Wrap(ErrInvalidToken)
	}
	return claims.UserID, nil
}
