// Package idgen — генерация идентификаторов (session_id и т.п. —
// zan-backend-tz-v3.md §2.1), внедряется как зависимость вместо
// uuid.New() напрямую — для детерминированных тестов.
package idgen

import "github.com/google/uuid"

// Generator — порт генерации идентификаторов. Возвращает string (не
// uuid.UUID) — domain-типы (internal/domain) сами хранят id как string
// в каноническом виде UUID и не импортируют google/uuid
// (BACKEND_CODING_STANDARDS.md §1.1), так что idgen отдаёт значение сразу
// в том виде, в котором его положат в domain.Session.ID и т.п.
type Generator interface {
	NewID() string
}

// UUIDGenerator — реализация Generator поверх google/uuid.
type UUIDGenerator struct{}

// NewID возвращает новый UUID v4 в каноническом строковом виде.
func (UUIDGenerator) NewID() string {
	return uuid.NewString()
}

// Fake — детерминированный Generator для юнит-тестов: возвращает ID
// каждый раз по очереди из заранее заданного списка.
type Fake struct {
	IDs []string
	n   int
}

// NewID возвращает следующий id из Fake.IDs. Паникует, если список
// исчерпан — сигнал, что тест сгенерировал больше сущностей, чем ожидал
// автор, а не штатный сценарий production-кода.
func (f *Fake) NewID() string {
	id := f.IDs[f.n]
	f.n++
	return id
}
