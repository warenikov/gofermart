package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/warenikov/gofermart/internal/domain"
	authsvc "github.com/warenikov/gofermart/internal/service/auth"
)

type fakeUserRepo struct {
	createFn     func(ctx context.Context, login, hash string) (*domain.User, error)
	getByLoginFn func(ctx context.Context, login string) (*domain.User, error)
	getByIDFn    func(ctx context.Context, id int64) (*domain.User, error)
}

func (f *fakeUserRepo) Create(ctx context.Context, login, hash string) (*domain.User, error) {
	return f.createFn(ctx, login, hash)
}

func (f *fakeUserRepo) GetByLogin(ctx context.Context, login string) (*domain.User, error) {
	return f.getByLoginFn(ctx, login)
}

func (f *fakeUserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	return f.getByIDFn(ctx, id)
}

type fakeTokens struct {
	issueFn func(userID int64) (string, error)
}

func (f *fakeTokens) Issue(userID int64) (string, error) {
	return f.issueFn(userID)
}

func hashPassword(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	return string(h)
}

func TestService_Register_Success(t *testing.T) {
	t.Parallel()
	var savedLogin, savedHash string
	repo := &fakeUserRepo{
		createFn: func(_ context.Context, login, hash string) (*domain.User, error) {
			savedLogin, savedHash = login, hash
			return &domain.User{ID: 7, Login: login, PasswordHash: hash}, nil
		},
	}
	tokens := &fakeTokens{issueFn: func(id int64) (string, error) {
		assert.Equal(t, int64(7), id)
		return "tok-7", nil
	}}

	svc := authsvc.NewService(repo, tokens, zap.NewNop())
	token, err := svc.Register(context.Background(), "alice", "secret")
	require.NoError(t, err)
	assert.Equal(t, "tok-7", token)
	assert.Equal(t, "alice", savedLogin)
	require.NotEmpty(t, savedHash)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(savedHash), []byte("secret")),
		"в репозиторий должен прийти валидный bcrypt-хеш")
}

func TestService_Register_EmptyInput(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{}
	tokens := &fakeTokens{}
	svc := authsvc.NewService(repo, tokens, zap.NewNop())

	cases := []struct{ login, password string }{
		{"", "x"}, {"x", ""}, {"", ""},
	}
	for _, c := range cases {
		_, err := svc.Register(context.Background(), c.login, c.password)
		require.Error(t, err)
		assert.True(t, errors.Is(err, authsvc.ErrInvalidInput),
			"login=%q password=%q → %v", c.login, c.password, err)
	}
}

func TestService_Register_LoginTaken(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{
		createFn: func(_ context.Context, _, _ string) (*domain.User, error) {
			return nil, domain.ErrLoginTaken
		},
	}
	svc := authsvc.NewService(repo, &fakeTokens{}, zap.NewNop())

	_, err := svc.Register(context.Background(), "alice", "x")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrLoginTaken))
}

func TestService_Login_Success(t *testing.T) {
	t.Parallel()
	hash := hashPassword(t, "secret")
	repo := &fakeUserRepo{
		getByLoginFn: func(_ context.Context, login string) (*domain.User, error) {
			assert.Equal(t, "alice", login)
			return &domain.User{ID: 7, Login: "alice", PasswordHash: hash}, nil
		},
	}
	tokens := &fakeTokens{issueFn: func(id int64) (string, error) {
		assert.Equal(t, int64(7), id)
		return "tok-7", nil
	}}
	svc := authsvc.NewService(repo, tokens, zap.NewNop())

	token, err := svc.Login(context.Background(), "alice", "secret")
	require.NoError(t, err)
	assert.Equal(t, "tok-7", token)
}

func TestService_Login_UnknownUser_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{
		getByLoginFn: func(_ context.Context, _ string) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
	}
	svc := authsvc.NewService(repo, &fakeTokens{}, zap.NewNop())

	_, err := svc.Login(context.Background(), "ghost", "x")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidCredentials))
	assert.False(t, errors.Is(err, domain.ErrUserNotFound),
		"факт отсутствия пользователя не должен утекать наружу")
}

func TestService_Login_WrongPassword_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()
	hash := hashPassword(t, "secret")
	repo := &fakeUserRepo{
		getByLoginFn: func(_ context.Context, _ string) (*domain.User, error) {
			return &domain.User{ID: 7, PasswordHash: hash}, nil
		},
	}
	svc := authsvc.NewService(repo, &fakeTokens{}, zap.NewNop())

	_, err := svc.Login(context.Background(), "alice", "wrong-password")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidCredentials))
}

func TestService_Login_EmptyInput(t *testing.T) {
	t.Parallel()
	svc := authsvc.NewService(&fakeUserRepo{}, &fakeTokens{}, zap.NewNop())

	_, err := svc.Login(context.Background(), "", "x")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidCredentials))
}
