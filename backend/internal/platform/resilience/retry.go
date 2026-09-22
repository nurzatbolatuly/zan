// Package resilience — общая реализация ретраев (backoff) и circuit
// breaker (sony/gobreaker), переиспользуемая internal/grpcclient и
// internal/agent (BACKEND_PLAN.md §1.1) — не переизобретается в каждом
// клиенте отдельно.
package resilience

import (
	"context"
	"time"
)

// RetryPolicy — экспоненциальный backoff без джиттера (внутренний трафик
// в приватной docker/VPC-сети, не публичный API — thundering herd на
// один helper/ за одним DNS-именем не тот риск, который здесь нужно
// гасить джиттером). MaxAttempts — общее число попыток, включая первую
// (MaxAttempts=1 — ретраев нет).
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

// Do вызывает fn, повторяя вызов, пока shouldRetry(err) верно и попытки не
// исчерпаны — с задержкой BaseDelay*2^(attempt-1) между попытками.
// Останавливается досрочно, если ctx отменён (не тратит оставшиеся попытки
// на заведомо неактуальный запрос). Возвращает ошибку последней попытки.
func (p RetryPolicy) Do(ctx context.Context, shouldRetry func(error) bool, fn func(ctx context.Context) error) error {
	maxAttempts := max(p.MaxAttempts, 1)

	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = fn(ctx)
		if err == nil || attempt == maxAttempts || !shouldRetry(err) {
			return err
		}

		delay := p.BaseDelay << (attempt - 1) //nolint:gosec // attempt мало (<=3), overflow недостижим
		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay):
		}
	}
	return err
}
