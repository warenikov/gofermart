// Package config загружает конфигурацию сервиса.
// Приоритет: дефолты → переменные окружения → флаги командной строки.
package config

import (
	"flag"
	"time"

	"github.com/caarlos0/env/v6"
)

// Config содержит параметры запуска сервиса.
type Config struct {
	// RunAddress — адрес и порт HTTP-сервера (флаг -a, env RUN_ADDRESS).
	RunAddress string `env:"RUN_ADDRESS"`
	// DatabaseURI — строка подключения к PostgreSQL (флаг -d, env DATABASE_URI).
	DatabaseURI string `env:"DATABASE_URI"`
	// AccrualSystemAddress — адрес системы расчёта начислений (флаг -r, env ACCRUAL_SYSTEM_ADDRESS).
	AccrualSystemAddress string `env:"ACCRUAL_SYSTEM_ADDRESS"`
	// LogLevel — уровень логирования: debug, info, warn, error (флаг -log-level, env LOG_LEVEL).
	LogLevel string `env:"LOG_LEVEL"`
	// LogFormat — формат логов: json или console (флаг -log-format, env LOG_FORMAT).
	LogFormat string `env:"LOG_FORMAT"`
	// JWTSecret — секрет для подписи JWT-токенов (флаг -jwt-secret, env JWT_SECRET).
	// Обязателен — без него сервис не стартует.
	JWTSecret string `env:"JWT_SECRET"`
	// JWTTTL — время жизни JWT-токена (флаг -jwt-ttl, env JWT_TTL).
	JWTTTL time.Duration `env:"JWT_TTL"`
}

// New собирает конфигурацию в три этапа:
//  1. Дефолтные значения
//  2. Переменные окружения (перекрывают дефолты)
//  3. Флаги командной строки (перекрывают всё)
func New() (*Config, error) {
	// 1. Дефолты
	cfg := &Config{
		RunAddress:           ":8080",
		DatabaseURI:          "",
		AccrualSystemAddress: "",
		LogLevel:             "info",
		LogFormat:            "console",
		JWTSecret:            "",
		JWTTTL:               24 * time.Hour,
	}

	// 2. Парсим конфиг из ОС
	err := parseEnv(cfg)

	if err != nil {
		return nil, err
	}

	// 3. Флаги командной строки
	flag.StringVar(&cfg.RunAddress, "a", cfg.RunAddress, "адрес и порт запуска сервиса")
	flag.StringVar(&cfg.DatabaseURI, "d", cfg.DatabaseURI, "адрес подключения к базе данных")
	flag.StringVar(&cfg.AccrualSystemAddress, "r", cfg.AccrualSystemAddress, "адрес системы расчёта начислений")
	flag.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "уровень логирования (debug/info/warn/error)")
	flag.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "формат логов (json/console)")
	flag.StringVar(&cfg.JWTSecret, "jwt-secret", cfg.JWTSecret, "секрет для подписи JWT")
	flag.DurationVar(&cfg.JWTTTL, "jwt-ttl", cfg.JWTTTL, "время жизни JWT-токена")
	flag.Parse()

	return cfg, nil
}

func parseEnv(cfg *Config) error {
	return env.Parse(cfg)
}
