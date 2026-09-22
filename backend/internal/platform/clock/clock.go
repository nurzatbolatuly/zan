// Package clock — источник текущего времени, внедряемый в service как
// зависимость, а не time.Now() напрямую — нужен для детерминированных
// тестов статус-машин/срока действия сессии (zan-backend-tz-v3.md §2.2).
package clock

import "time"

// Clock — порт источника времени.
type Clock interface {
	Now() time.Time
}

// Real — реализация Clock поверх time.Now().
type Real struct{}

// Now возвращает текущее время.
func (Real) Now() time.Time {
	return time.Now()
}

// Fake — детерминированный Clock для юнит-тестов бизнес-логики, зависящей
// от времени (статус-машины, срок действия сессии —
// BACKEND_CODING_STANDARDS.md §10: "тестируется поведение, а не
// реализация", без реального time.Sleep в тестах).
type Fake struct {
	T time.Time
}

// Now возвращает зафиксированное в Fake время.
func (f Fake) Now() time.Time {
	return f.T
}
