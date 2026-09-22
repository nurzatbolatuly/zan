package resilience

import (
	"context"
	"errors"
	"time"

	"github.com/sony/gobreaker"
)

// ErrBreakerOpen — helper/ недоступен целиком (серия отказов подряд), новые
// вызовы отклоняются немедленно вместо долгого ожидания таймаута
// (BACKEND_PLAN.md §3.3: "UNAVAILABLE — Python недоступен целиком —
// открывает circuit breaker"; zan-backend-tz-v3.md §5.6: "все новые треды
// сразу уходят в error без долгого ожидания"). Обёртка над
// gobreaker.ErrOpenState/ErrTooManyRequests — вызывающий код
// (internal/grpcclient) проверяет errors.Is(err, ErrBreakerOpen), не
// заглядывает во внутренний тип gobreaker.
var ErrBreakerOpen = errors.New("resilience: circuit breaker is open")

// BreakerConfig — настройки, за которые отвечает вызывающий код
// (internal/grpcclient конфигурирует один Breaker на весь клиент к
// helper/, а не по одному на метод — отказы FilesService и SttService
// одного и того же процесса Python значат одно и то же: сервис лежит).
type BreakerConfig struct {
	// Name — только для логов/метрик (gobreaker.Settings.Name).
	Name string
	// ConsecutiveFailuresToTrip — сколько подряд неудач открывают breaker.
	ConsecutiveFailuresToTrip uint32
	// OpenTimeout — сколько breaker остаётся открытым, прежде чем пропустить
	// одну пробную попытку (half-open).
	OpenTimeout time.Duration
}

// Breaker — тонкая обёртка над sony/gobreaker.CircuitBreaker: сужает
// сигнатуру Execute до func(context.Context) error (никому из вызывающего
// кода не нужен интерфейс interface{}, который несёт исходный gobreaker,
// см. BACKEND_CODING_STANDARDS.md §1.1 — порт под конкретную задачу, не
// произвольный generic-контейнер) и маппит открытое состояние на пакетный
// сентинел ErrBreakerOpen.
type Breaker struct {
	cb *gobreaker.CircuitBreaker
}

// NewBreaker строит Breaker с заданными настройками.
func NewBreaker(cfg BreakerConfig) *Breaker {
	settings := gobreaker.Settings{
		Name:    cfg.Name,
		Timeout: cfg.OpenTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.ConsecutiveFailuresToTrip
		},
	}
	return &Breaker{cb: gobreaker.NewCircuitBreaker(settings)}
}

// Execute вызывает fn, если breaker закрыт/полуоткрыт; если breaker открыт —
// возвращает ErrBreakerOpen, не вызывая fn.
func (b *Breaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	_, err := b.cb.Execute(func() (any, error) {
		return nil, fn(ctx)
	})
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return ErrBreakerOpen
	}
	return err
}
