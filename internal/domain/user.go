package domain

import "time"

// User — учётная запись пользователя сервиса лояльности.
type User struct {
	ID           int64
	Login        string
	PasswordHash string
	CreatedAt    time.Time
}
