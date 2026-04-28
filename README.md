# Гофермарт — накопительная система лояльности

HTTP API сервис для управления баллами лояльности пользователей интернет-магазина.

## Требования

- Go 1.26+
- Docker и Docker Compose
- golangci-lint (`brew install golangci-lint`)
- swag (`go install github.com/swaggo/swag/cmd/swag@latest`)

## Локальный запуск

### Через Docker Compose

```bash
docker compose up -d
```

Сервис будет доступен на `http://localhost:8080`.

### Без Docker

1. Запустите PostgreSQL:

```bash
docker compose up db -d
```

2. Запустите сервис:

```bash
make run
# или с параметрами:
go run ./cmd/gophermart -a :8080 -d "postgresql://postgres:postgres@localhost:5432/gofermart?sslmode=disable" -r "http://localhost:8081" -log-format console -log-level debug
```

## Переменные окружения

| Переменная               | Флаг          | По умолчанию | Описание                          |
|--------------------------|---------------|--------------|-----------------------------------|
| `RUN_ADDRESS`            | `-a`          | `:8080`      | Адрес и порт запуска сервиса      |
| `DATABASE_URI`           | `-d`          | —            | Строка подключения к PostgreSQL   |
| `ACCRUAL_SYSTEM_ADDRESS` | `-r`          | —            | Адрес системы расчёта начислений  |
| `LOG_LEVEL`              | `-log-level`  | `info`       | Уровень логирования               |
| `LOG_FORMAT`             | `-log-format` | `json`       | Формат логов (`json` / `console`) |

## Тестирование

### Юнит-тесты

```bash
make test-unit
```

### Интеграционные тесты

Требуется запущенный Docker.

```bash
make test-integration
```

### Все тесты

```bash
make test-all
```

## Тестирование с accrual-сервисом

1. Запустите PostgreSQL:

```bash
docker compose up db -d
```

2. Запустите accrual-сервис:

```bash
make accrual-start
```

3. Наполните accrual тестовыми данными:

```bash
make seed
```

4. Запустите основной сервис:

```bash
make run
```

Теперь можно отправлять заказы с номерами из `testdata/seeds/orders/` и проверять начисление баллов.

**Тестовые номера заказов (валидные по алгоритму Луна):**
- `79927398713`
- `49927398716`
- `12345678903`

## Линтер

```bash
make lint
```

## Swagger

```bash
make swagger
```

Документация будет доступна на `http://localhost:8080/swagger/`.

## Сборка

```bash
make build
# бинарник: bin/gophermart
```
