//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/warenikov/gofermart/internal/domain"
	"github.com/warenikov/gofermart/internal/repository"
)

func TestUserRepository_Create_GetByLogin_GetByID(t *testing.T) {
	resetDB(t)
	repo := repository.NewUserRepository(testPool)
	ctx := context.Background()

	created, err := repo.Create(ctx, "alice", "hashed-password")
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	assert.Equal(t, "alice", created.Login)
	assert.Equal(t, "hashed-password", created.PasswordHash)
	assert.False(t, created.CreatedAt.IsZero())

	byLogin, err := repo.GetByLogin(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, created.ID, byLogin.ID)

	byID, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "alice", byID.Login)
}

func TestUserRepository_Create_DuplicateLogin(t *testing.T) {
	resetDB(t)
	repo := repository.NewUserRepository(testPool)
	ctx := context.Background()

	_, err := repo.Create(ctx, "bob", "hash1")
	require.NoError(t, err)

	_, err = repo.Create(ctx, "bob", "hash2")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrLoginTaken),
		"ожидается ErrLoginTaken, получено: %v", err)
}

func TestUserRepository_GetByLogin_NotFound(t *testing.T) {
	resetDB(t)
	repo := repository.NewUserRepository(testPool)

	_, err := repo.GetByLogin(context.Background(), "nobody")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrUserNotFound))
}

func TestUserRepository_GetByID_NotFound(t *testing.T) {
	resetDB(t)
	repo := repository.NewUserRepository(testPool)

	_, err := repo.GetByID(context.Background(), 99999)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrUserNotFound))
}
