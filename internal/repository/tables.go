package repository

// Имена таблиц БД. Используются в SQL-запросах через конкатенацию const-строк,
// чтобы переименование таблицы требовало правки только в одном месте.
// Константы экспортированы, чтобы их же могли использовать интеграционные тесты
// для TRUNCATE / setup и не дублировать имена.
const (
	TableUsers       = "users"
	TableOrders      = "orders"
	TableWithdrawals = "withdrawals"
)
