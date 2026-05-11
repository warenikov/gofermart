// Package auth — сервисный слой регистрации и аутентификации пользователей.
package auth

import (
	"context"
	"errors"

	"github.com/samber/oops"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/warenikov/gofermart/internal/domain"
	"github.com/warenikov/gofermart/internal/logger"
)

// TokenIssuer выпускает аутентификационные токены.
type TokenIssuer interface {
	Issue(userID int64) (string, error)
}

// Service — регистрация и логин с bcrypt-хешированием паролей и JWT-токенами.
type Service struct {
	users      domain.UserRepository
	tokens     TokenIssuer
	bcryptCost int
	log        *zap.Logger
}

// NewService собирает сервис с дефолтным bcrypt cost.
func NewService(users domain.UserRepository, tokens TokenIssuer, log *zap.Logger) *Service {
	return &Service{
		users:      users,
		tokens:     tokens,
		bcryptCost: bcrypt.DefaultCost,
		log:        logger.For(log, "service.auth"),
	}
}

// ErrInvalidInput — пустой логин или пароль во входных данных.
var ErrInvalidInput = errors.New("login and password are required")

// Register регистрирует пользователя, хеширует пароль и сразу выпускает токен.
// Возможные доменные ошибки: domain.ErrLoginTaken.
func (s *Service) Register(ctx context.Context, login, password string) (string, error) {
	if login == "" || password == "" {
		return "", oops.In("service.auth").Code("invalid_input").Wrap(ErrInvalidInput)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return "", oops.In("service.auth").Code("hash").Wrapf(err, "захешировать пароль")
	}

	user, err := s.users.Create(ctx, login, string(hash))
	if err != nil {
		return "", err
	}

	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return "", oops.In("service.auth").Code("issue_token").With("user_id", user.ID).
			Wrapf(err, "выпустить токен после регистрации")
	}
	return token, nil
}

// Login проверяет пару логин/пароль и возвращает токен.
// На любую ошибку аутентификации (отсутствие юзера, неверный пароль, пустой ввод)
// возвращает domain.ErrInvalidCredentials, чтобы не выдавать наличие логина.
func (s *Service) Login(ctx context.Context, login, password string) (string, error) {
	if login == "" || password == "" {
		return "", oops.In("service.auth").Code("invalid_input").
			Wrap(domain.ErrInvalidCredentials)
	}

	user, err := s.users.GetByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return "", oops.In("service.auth").Code("user_not_found").
				With("login", login).Wrap(domain.ErrInvalidCredentials)
		}
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", oops.In("service.auth").Code("password_mismatch").
			With("user_id", user.ID).Wrap(domain.ErrInvalidCredentials)
	}

	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return "", oops.In("service.auth").Code("issue_token").With("user_id", user.ID).
			Wrapf(err, "выпустить токен при логине")
	}
	return token, nil
}
