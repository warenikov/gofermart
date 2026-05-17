// Package domain содержит бизнес-сущности, доменные ошибки и интерфейсы репозиториев.
package domain

import "errors"

var (
	// ErrLoginTaken — логин уже занят при регистрации.
	ErrLoginTaken = errors.New("login already taken")
	// ErrInvalidCredentials — неверная пара логин/пароль.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUserNotFound — пользователь не найден.
	ErrUserNotFound = errors.New("user not found")

	// ErrInvalidLuhn — номер заказа не прошёл проверку алгоритмом Луна.
	ErrInvalidLuhn = errors.New("invalid order number")
	// ErrOrderAlreadyOwned — заказ уже загружен этим же пользователем.
	ErrOrderAlreadyOwned = errors.New("order already owned by current user")
	// ErrOrderOwnedByOther — заказ принадлежит другому пользователю.
	ErrOrderOwnedByOther = errors.New("order owned by another user")
	// ErrOrderNotFound — заказ не найден.
	ErrOrderNotFound = errors.New("order not found")

	// ErrInsufficientFunds — на счёте пользователя недостаточно средств.
	ErrInsufficientFunds = errors.New("insufficient funds")
)
