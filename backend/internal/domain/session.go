// Package domain содержит чистые бизнес-типы, общие для internal/service.
// Ничего, кроме стандартной библиотеки и других пакетов domain, сюда не
// импортируется — ни gin, ни pgx, ни grpc, ни даже google/uuid
// (BACKEND_CODING_STANDARDS.md §1.1) — поэтому идентификаторы здесь
// хранятся как string в каноническом виде UUID, а не uuid.UUID; парсинг/
// валидация формата — забота адаптеров (internal/repo, internal/service),
// не домена.
package domain

import (
	"fmt"
	"time"
)

// Language — предпочитаемый язык сессии (zan-backend-tz-v3.md §2.2).
type Language string

const (
	LanguageRu Language = "ru"
	LanguageKz Language = "kz"
)

// ParseLanguage валидирует сырое значение (из запроса/БД) как Language.
func ParseLanguage(s string) (Language, error) {
	switch v := Language(s); v {
	case LanguageRu, LanguageKz:
		return v, nil
	default:
		return "", fmt.Errorf("domain: invalid language %q", s)
	}
}

// Theme — сохранённое предпочтение темы, nullable (zan-backend-tz-v3.md §2.2).
type Theme string

const (
	ThemeLight Theme = "light"
	ThemeDark  Theme = "dark"
)

// ParseTheme валидирует сырое значение (из запроса/БД) как Theme.
func ParseTheme(s string) (Theme, error) {
	switch v := Theme(s); v {
	case ThemeLight, ThemeDark:
		return v, nil
	default:
		return "", fmt.Errorf("domain: invalid theme %q", s)
	}
}

// Session — анонимная сессия пользователя, заменяет User целиком
// (opaque-токен вместо авторизации, zan-backend-tz-v3.md §2.2).
type Session struct {
	ID             string
	Language       Language
	Theme          *Theme
	OnboardingSeen bool
	CreatedAt      time.Time
	LastSeenAt     time.Time
	ExpiresAt      time.Time
}

// IsExpired — истекло ли скользящее окно сессии на момент now
// (zan-backend-tz-v3.md §2.1, §5.5).
func (s Session) IsExpired(now time.Time) bool {
	return now.After(s.ExpiresAt)
}
