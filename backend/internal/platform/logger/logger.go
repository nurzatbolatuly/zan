// Package logger — единственная точка настройки структурированного
// логирования в backend/ (BACKEND_CODING_STANDARDS.md §4.1). Формат записи
// и уровни — контракт backend-roadmap.md §6.1/§6.4:
//
//	{
//	  "timestamp": "...",
//	  "level": "INFO|WARN|ERROR|CRITICAL|DEBUG",
//	  "service": "go-core",
//	  "trace_id": "...",
//	  "message": "...",
//	  "context": { ... }
//	}
//
// "session_id"/"stage" — не общие поля хендлера, а добавляются через
// logger.With(...) в тех местах, где эти понятия появляются (Session —
// Stage 1, статус-машина треда — Stage 3): на Stage 0 их ещё физически
// неоткуда взять, подставлять пустые значения было бы ложью в логах.
package logger

import (
	"context"
	"io"
	"log/slog"
)

// ServiceName — значение поля "service" для всех записей backend, как
// зафиксировано в backend-roadmap.md §6.1 (буквально "go-core", не
// "backend").
const ServiceName = "go-core"

// LevelCritical — пятый уровень из каталога backend-roadmap.md §6.4,
// которого нет среди встроенных уровней slog (Debug/Info/Warn/Error).
// Используется для инфраструктурных сбоев уровня всего сервиса и для
// восстановленных паник (см. httpserver.Recovery).
const LevelCritical = slog.Level(12) // выше slog.LevelError (8), см. пакет log/slog

// New строит JSON-логгер по контракту §6.1 и пишет в w (в проде — os.Stdout,
// в тестах — любой io.Writer, которым можно проверить фактический вывод).
// Переименовывает стандартные slog-поля "time"/"msg" в "timestamp"/"message"
// и печатает LevelCritical как "CRITICAL" вместо стандартного "ERROR+4"
// (рекомендованный slog-паттерн для кастомных уровней, см. log/slog godoc,
// "Custom levels"). Домен-специфичные поля вызывающий код группирует через
// slog.Group("context", ...), чтобы они легли в контракт как вложенный
// объект "context", а не расползлись по верхнему уровню записи.
func New(w io.Writer, level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: replaceAttr,
	})
	return slog.New(handler).With(slog.String("service", ServiceName))
}

func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.TimeKey:
		a.Key = "timestamp"
	case slog.MessageKey:
		a.Key = "message"
	case slog.LevelKey:
		if lvl, ok := a.Value.Any().(slog.Level); ok && lvl == LevelCritical {
			a.Value = slog.StringValue("CRITICAL")
		}
	}
	return a
}

// ParseLevel переводит строковое значение LOG_LEVEL (config.Config.LogLevel)
// в slog.Level. Невалидное значение — ошибка конфигурации, не тихий фолбэк
// на info (что-то не так с деплоем, если сюда пришла опечатка).
func ParseLevel(s string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return 0, err
	}
	return level, nil
}

type ctxKey struct{}

// WithContext кладёт логгер в context.Context — так request-scoped поля
// (trace_id и далее session_id), добавленные один раз в middleware, видны
// всем нижележащим вызовам без повторного сбора полей на каждом уровне.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext достаёт логгер, положенный WithContext. Если в контексте
// логгера нет (тесты, фоновые задачи вне HTTP-запроса) — возвращает
// slog.Default() как безопасный фолбэк, а не паникует.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

type traceIDCtxKey struct{}

// WithTraceID кладёт сырое значение trace_id в context.Context — отдельно
// от логгера (WithContext), потому что internal/grpcclient нужен сам
// строковый trace_id, чтобы положить его в исходящую gRPC-метадату
// (x-trace-id, BACKEND_PLAN.md §3.3), а не только видеть его как поле уже
// собранной записи лога, откуда значение обратно не достать.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDCtxKey{}, traceID)
}

// TraceIDFromContext достаёт trace_id, положенный WithTraceID. Пустая
// строка + false — контекст вне HTTP-запроса (тесты, фоновые задачи).
func TraceIDFromContext(ctx context.Context) (string, bool) {
	traceID, ok := ctx.Value(traceIDCtxKey{}).(string)
	return traceID, ok
}
