package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/warenikov/gofermart/internal/domain"
)

func TestValidateLuhn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"валидный — 12345678903 (тестовый номер из спеки)", "12345678903", true},
		{"валидный — 79927398713", "79927398713", true},
		{"валидный — короткий 18", "18", true},
		{"валидный — длинный 49927398716", "49927398716", true},
		{"невалидный — 12345678901", "12345678901", false},
		{"невалидный — 79927398710", "79927398710", false},
		{"невалидный — пустая строка", "", false},
		{"невалидный — пробел внутри", "12345 78903", false},
		{"невалидный — буква внутри", "1234567890a", false},
		{"вырожденный — только нули, формально валиден", "0000", true},
		{"невалидный — отрицательный знак", "-12345678903", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, domain.ValidateLuhn(tt.in))
		})
	}
}
