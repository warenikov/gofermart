// Package logger предоставляет фабрику zap-логгеров для каждого слоя приложения.
// Поддерживает форматы JSON (для продакшена) и console (для разработки).
// Ошибки samber/oops разворачиваются в структурированные поля логгером через [Err].
package logger

import (
	"fmt"

	oopszap "github.com/samber/oops/loggers/zap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New создаёт корневой логгер с указанным уровнем и форматом.
// level: debug | info | warn | error
// format: json | console
func New(level, format string) (*zap.Logger, error) {
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return nil, fmt.Errorf("неверный уровень логирования %q: %w", level, err)
	}

	var cfg zap.Config
	if format == "console" {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
	}
	cfg.Level = lvl

	log, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("ошибка создания логгера: %w", err)
	}

	return log, nil
}

// For возвращает дочерний логгер с предустановленным полем layer.
// Используется при инициализации каждого слоя приложения.
func For(base *zap.Logger, layer string) *zap.Logger {
	return base.With(zap.String("layer", layer))
}

// Err оборачивает ошибку для структурированного логирования.
// Если ошибка построена через samber/oops — её поля (.In, .Code, .Tags, .With) разворачиваются
// в объект "error" вместе со стектрейсом. Обычные ошибки логируются как поле message.
func Err(err error) zap.Field {
	if err == nil {
		return zap.Skip()
	}
	return zap.Inline(zapcore.ObjectMarshalerFunc(func(enc zapcore.ObjectEncoder) error {
		if e := oopszap.OopsMarshalFunc(err); e != nil {
			_ = enc.AddObject("error", e)
		}
		if st := oopszap.OopsStackMarshaller(err); st != "" {
			enc.AddString("stacktrace", st)
		}
		return nil
	}))
}
