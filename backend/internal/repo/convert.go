package repo

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// parseUUID/fromPgUUID — конвертация domain-id (string, каноническая
// строка UUID — internal/domain намеренно не импортирует google/uuid,
// BACKEND_CODING_STANDARDS.md §1.1) в/из pgtype.UUID, которым говорит
// сгенерированный sqlc-код. Общие хелперы для всех *_repo.go в этом
// пакете, не дублируются на каждую таблицу.

func parseUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("repo: parse uuid %q: %w", id, err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func fromPgUUID(id pgtype.UUID) string {
	return uuid.UUID(id.Bytes).String()
}

func toTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// toInt32 — явная проверка диапазона перед int -> int32 (gosec G115:
// "integer overflow conversion"). Money/qty/discount-поля приходят уже
// провалидированными business-слоем (цена >= 0, qty 1–20, discount 0–90 —
// internal/service/catalog), но репозиторий не должен молча полагаться на
// это в конвертации типов, поэтому проверяет диапазон сам.
func toInt32(n int) (int32, error) {
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, fmt.Errorf("repo: value %d overflows int32", n)
	}
	return int32(n), nil
}
