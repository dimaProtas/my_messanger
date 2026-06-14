package user

import "strings"

// MaskPhone маскирует номер телефона для отображения в результатах поиска/списках контактов.
//
// Правила:
//   - Если номер короче 6 символов — заменяется звёздочками целиком.
//   - Иначе: +<первые 4 цифры>*****<последние 2 цифры>.
//
// Пример: "79161234567" → "+7916*****67".
func MaskPhone(phone string) string {
	p := strings.TrimPrefix(phone, "+")
	if len(p) < 6 {
		return "+" + strings.Repeat("*", len(p))
	}
	return "+" + p[:4] + strings.Repeat("*", 5) + p[len(p)-2:]
}
