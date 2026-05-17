package repository

// Имена таблиц БД. Используются в SQL-запросах через конкатенацию const-строк,
// чтобы переименование таблицы требовало правки только в одном месте.
const (
	tableUsers       = "users"
	tableOrders      = "orders"
	tableWithdrawals = "withdrawals"
)
