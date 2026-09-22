// Package clientlog — POST /logs/client (BACKEND_PLAN.md Stage 7,
// zan-backend-tz-v3.md §6.3): связывает клиентские события (отказ в
// доступе к микрофону, неудавшийся вызов API, необработанное исключение в
// интерфейсе) с серверной цепочкой логов по trace_id/session_id. Ничего не
// пишет в БД — событие целиком уходит в тот же JSON-лог на stdout, что и
// остальные записи backend'а (backend-roadmap.md §6.1), собирается тем же
// агрегатором (Loki/ELK/аналог).
package clientlog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"zan-backend/internal/platform/logger"
)

// Level — уровень клиентского события. Ровно три значения по
// zan-backend-tz-v3.md §6.3 ("level": "INFO|WARN|ERROR") — не весь набор
// backend-roadmap.md §6.4 (DEBUG — только dev-детали промптов, CRITICAL —
// инфраструктурный сбой всего сервиса, ни одно из двух не может прийти с
// фронта осмысленно).
type Level string

const (
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

// ParseLevel валидирует сырое значение из тела запроса как Level.
func ParseLevel(s string) (Level, error) {
	switch v := Level(s); v {
	case LevelInfo, LevelWarn, LevelError:
		return v, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidLevel, s)
	}
}

var (
	// ErrInvalidLevel — level вне INFO/WARN/ERROR.
	ErrInvalidLevel = errors.New("clientlog: invalid level")
	// ErrEmptyEvent — event пуст после trim (машиночитаемое имя события,
	// "mic_permission_denied"/"api_call_failed"/... — без него запись в
	// логе бесполезна для фильтрации).
	ErrEmptyEvent = errors.New("clientlog: event must not be empty")
	// ErrEmptyMessage — message пуст после trim (человекочитаемое описание,
	// zan-backend-tz-v3.md §6.3 — обязательное поле тела запроса).
	ErrEmptyMessage = errors.New("clientlog: message must not be empty")
)

// Entry — тело POST /logs/client (zan-backend-tz-v3.md §6.3). TraceID —
// НЕ trace_id текущего HTTP-запроса (тот уже в context.Context, положен
// RequestContext middleware на этот самый вызов /logs/client) — это
// trace_id ДРУГОГО, уже завершённого запроса/действия на фронте, к
// которому относится событие, поэтому логируется отдельным полем
// (context.client_trace_id), не подменяет собой ambient trace_id записи.
type Entry struct {
	SessionID string
	TraceID   string
	Level     Level
	Event     string
	Message   string
	Context   map[string]any
}

// Service — бизнес-логика клиентских логов: валидация + запись через
// единый логгер процесса (internal/platform/logger), без состояния и без
// сети — тот же принцип "каждая область — свой internal/service/X", что и
// у internal/service/analytics (BACKEND_CODING_STANDARDS.md §1.1), даже
// когда правил валидации немного.
type Service struct{}

// New собирает Service.
func New() *Service {
	return &Service{}
}

// Log — POST /logs/client: валидирует Entry и пишет одну структурированную
// запись на том уровне, который прислал клиент (Message — из тела запроса,
// не текст самого backend'а, поэтому не проходит через "не логировать
// пользовательский текст на INFO+" — это уже сообщение О логировании, а не
// пользовательский ввод, обрабатываемый агентом, BACKEND_CODING_STANDARDS.md §4.4).
func (s *Service) Log(ctx context.Context, e Entry) error {
	if strings.TrimSpace(e.Event) == "" {
		return ErrEmptyEvent
	}
	if strings.TrimSpace(e.Message) == "" {
		return ErrEmptyMessage
	}

	l := logger.FromContext(ctx)
	if e.SessionID != "" {
		l = l.With(slog.String("session_id", e.SessionID))
	}

	l.Log(ctx, slogLevel(e.Level), "client_event", slog.Group("context",
		slog.String("event", e.Event),
		slog.String("client_message", e.Message),
		slog.String("client_trace_id", e.TraceID),
		slog.Any("client_context", e.Context),
	))
	return nil
}

func slogLevel(l Level) slog.Level {
	switch l {
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
