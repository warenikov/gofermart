package domain

// ValidateLuhn проверяет строку цифр алгоритмом Луна.
// Возвращает true, если строка непустая, состоит только из цифр и контрольная сумма делится на 10.
func ValidateLuhn(s string) bool {
	if s == "" {
		return false
	}
	sum := 0
	double := false
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
		digit := int(c - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	return sum%10 == 0
}
