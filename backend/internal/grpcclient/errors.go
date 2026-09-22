package grpcclient

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"zan-backend/internal/platform/resilience"
)

func shouldRetry(err error) bool {
	return status.Code(err) == codes.DeadlineExceeded
}

// IsUnavailable — helper/ недоступен целиком (breaker открыт) или не
// ответил вовремя даже после ретраев (BACKEND_PLAN.md §3.3, zan-backend-tz-v3.md
// §5.6: "сервис временно недоступен, попробуйте позже"). Вызывающий код
// (internal/service/file, internal/httpserver) не должен различать breaker
// vs таймаут дальше этой точки — пользователю в обоих случаях один и тот же
// нейтральный ответ.
func IsUnavailable(err error) bool {
	if errors.Is(err, resilience.ErrBreakerOpen) {
		return true
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}

// IsInvalidArgument — Python отклонил запрос как заведомо необрабатываемый
// (неподдерживаемый MIME, повреждённый файл, пустая транскрипция —
// BACKEND_PLAN.md §3.3: "INVALID_ARGUMENT — невалидный запрос — не
// ретраится"). Не ошибка инфраструктуры — вызывающий код мапит её на
// конкретный бизнес-исход (processing_status=error, "не расслышал" и т.п.),
// не на общий internal_error/503.
func IsInvalidArgument(err error) bool {
	return status.Code(err) == codes.InvalidArgument
}
