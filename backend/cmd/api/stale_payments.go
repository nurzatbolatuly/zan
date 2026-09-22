package main

import (
	"context"
	"log/slog"
	"time"

	"zan-backend/internal/service/billing"
)

// stalePaymentCheckInterval — как часто фоновая задача проверяет
// pending-платежи на возраст (BACKEND_PLAN.md, Stage 2: "goroutine +
// time.Ticker"). Не задано продуктом — рекомендация архитектора: заметно
// чаще, чем сам порог (billing.StalePendingAfter, 30 минут), чтобы
// "зависшие" платежи не висели в pending намного дольше порога, но не
// настолько часто, чтобы создавать лишнюю нагрузку на БД пустыми проверками.
const stalePaymentCheckInterval = time.Minute

// runStalePaymentExpirer — периодически помечает pending-платежи старше
// billing.StalePendingAfter как failed (zan-backend-tz-v3.md §5.1).
// Останавливается по отмене ctx (тот же сигнал graceful shutdown, что и у
// HTTP-сервера) — не отдельный процесс, который нужно гасить особо.
func runStalePaymentExpirer(ctx context.Context, l *slog.Logger, svc *billing.Service) {
	ticker := time.NewTicker(stalePaymentCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := svc.ExpireStalePending(ctx)
			if err != nil {
				l.Error("stale_payments_expire_failed", slog.String("error", err.Error()))
				continue
			}
			if n > 0 {
				l.Info("stale_payments_expired", slog.Int("count", n))
			}
		}
	}
}
