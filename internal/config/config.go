// Package config загружает конфигурацию сервиса из флагов командной строки и переменных окружения.
// Флаги имеют приоритет над переменными окружения.
package config

import (
	"flag"
	"os"
)

// Config содержит параметры запуска сервиса.
type Config struct {
	// RunAddress — адрес и порт HTTP-сервера (флаг -a, env RUN_ADDRESS).
	RunAddress string
	// DatabaseURI — строка подключения к PostgreSQL (флаг -d, env DATABASE_URI).
	DatabaseURI string
	// AccrualSystemAddress — адрес системы расчёта начислений (флаг -r, env ACCRUAL_SYSTEM_ADDRESS).
	AccrualSystemAddress string
	// LogLevel — уровень логирования: debug, info, warn, error (флаг -log-level, env LOG_LEVEL).
	LogLevel string
	// LogFormat — формат логов: json или console (флаг -log-format, env LOG_FORMAT).
	LogFormat string
}

// New читает конфигурацию: сначала переменные окружения, затем флаги (флаги имеют приоритет).
func New() *Config {
	cfg := &Config{
		RunAddress:           getEnv("RUN_ADDRESS", ":8080"),
		DatabaseURI:          getEnv("DATABASE_URI", ""),
		AccrualSystemAddress: getEnv("ACCRUAL_SYSTEM_ADDRESS", ""),
		LogLevel:             getEnv("LOG_LEVEL", "info"),
		LogFormat:            getEnv("LOG_FORMAT", "json"),
	}

	flag.StringVar(&cfg.RunAddress, "a", cfg.RunAddress, "адрес и порт запуска сервиса")
	flag.StringVar(&cfg.DatabaseURI, "d", cfg.DatabaseURI, "адрес подключения к базе данных")
	flag.StringVar(&cfg.AccrualSystemAddress, "r", cfg.AccrualSystemAddress, "адрес системы расчёта начислений")
	flag.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "уровень логирования (debug/info/warn/error)")
	flag.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "формат логов (json/console)")
	flag.Parse()

	return cfg
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
