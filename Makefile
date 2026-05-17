BINARY     := bin/gophermart
MAIN       := ./cmd/gophermart

UNAME_S    := $(shell uname -s)
UNAME_M    := $(shell uname -m)
ifeq ($(UNAME_S),Darwin)
    ifeq ($(UNAME_M),arm64)
        ACCRUAL_BINARY := cmd/accrual/accrual_darwin_arm64
    else
        ACCRUAL_BINARY := cmd/accrual/accrual_darwin_amd64
    endif
else
    ACCRUAL_BINARY := cmd/accrual/accrual_linux_amd64
endif

ACCRUAL_HOST := localhost
ACCRUAL_PORT := 8081
ACCRUAL_URL  := http://$(ACCRUAL_HOST):$(ACCRUAL_PORT)

TEST_DB_URI  := postgresql://postgres:postgres@localhost:5433/gofermart_test?sslmode=disable

.PHONY: all build run clean \
        docker-up docker-down docker-build \
        accrual-start \
        seed seed-goods seed-orders \
        test-unit test-integration test-all \
        mocks lint swagger help

all: build

## build: собрать бинарник сервиса
build:
	go build -buildvcs=false -o $(BINARY) $(MAIN)

## run: запустить сервис локально
run:
	go run $(MAIN) -a :8080 -log-format console -log-level debug

## clean: удалить артефакты сборки
clean:
	rm -rf bin/ docs/

## docker-build: собрать Docker-образ
docker-build:
	docker compose build

## docker-up: поднять app + postgres в Docker
docker-up:
	docker compose up -d

## docker-down: остановить и удалить контейнеры
docker-down:
	docker compose down -v

## accrual-start: запустить бинарь accrual локально (хранит данные in-memory, БД не нужна)
accrual-start:
	@chmod +x $(ACCRUAL_BINARY)
	$(ACCRUAL_BINARY) -a $(ACCRUAL_HOST):$(ACCRUAL_PORT)

## seed-goods: зарегистрировать механики вознаграждений в accrual
seed-goods:
	@echo "→ Регистрируем механики вознаграждений..."
	@for f in testdata/seeds/goods/*.json; do \
		echo "  POST /api/goods < $$f"; \
		curl -s -o /dev/null -w "  статус: %{http_code}\n" \
			-X POST $(ACCRUAL_URL)/api/goods \
			-H "Content-Type: application/json" \
			-d @$$f; \
	done

## seed-orders: зарегистрировать тестовые заказы в accrual
seed-orders:
	@echo "→ Регистрируем тестовые заказы в accrual..."
	@for f in testdata/seeds/orders/*.json; do \
		echo "  POST /api/orders < $$f"; \
		curl -s -o /dev/null -w "  статус: %{http_code}\n" \
			-X POST $(ACCRUAL_URL)/api/orders \
			-H "Content-Type: application/json" \
			-d @$$f; \
	done

## seed: полный сид данных (goods + orders)
seed: seed-goods seed-orders

## test-unit: запустить юнит-тесты
test-unit:
	go test -race -count=1 ./...

## test-integration: поднять тестовое окружение и запустить интеграционные тесты
test-integration:
	docker compose -f docker-compose.test.yml up -d
	@echo "Ждём готовности PostgreSQL..."
	@until docker compose -f docker-compose.test.yml exec -T db pg_isready -U postgres; do sleep 1; done
	DATABASE_URI="$(TEST_DB_URI)" go test -race -count=1 -p=1 -tags=integration ./...
	docker compose -f docker-compose.test.yml down -v

## test-all: запустить все тесты
test-all: test-unit test-integration

## mocks: сгенерировать моки по конфигу .mockery.yml
mocks:
	go tool mockery

## lint: запустить линтер
lint:
	golangci-lint run ./...

## swagger: сгенерировать документацию Swagger
swagger:
	swag init -g cmd/gophermart/main.go -o docs

## help: показать список команд
help:
	@grep -E '^## ' Makefile | sed 's/## //'
