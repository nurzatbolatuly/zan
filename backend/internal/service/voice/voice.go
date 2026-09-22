// Package voice — юзкейс POST /voice/transcribe (zan-backend-tz-v2.md §4.7):
// синхронное распознавание речи, без персистентности (аудио не привязано ни
// к какому Thread/Message — "обработка аудио как есть — вне скоупа").
package voice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/platform/logger"
)

var (
	// ErrUnsupportedMimeType — zan-backend-tz-v3.md §5.4 (тот же принцип
	// каталога ошибок, что у file.ErrUnsupportedMimeType).
	ErrUnsupportedMimeType = errors.New("voice: unsupported mime type")

	// ErrTooLarge — короткая голосовая заметка, не полноценный файл.
	ErrTooLarge = errors.New("voice: too large")

	// ErrEmptyTranscript — STT вернул пустой текст (тишина),
	// zan-backend-tz-v3.md §5.4: "не расслышал, повторите ещё раз" — Go не
	// создаёт сообщение.
	ErrEmptyTranscript = errors.New("voice: empty transcript")
)

// AllowedMimeTypes — форматы, которые реально отдаёт MediaRecorder
// современных браузеров (webm/opus по умолчанию в Chrome/Firefox, mp4/aac
// в Safari) плюс типовые контейнеры для загрузки готового файла.
var AllowedMimeTypes = map[string]string{
	"audio/webm": "webm",
	"audio/ogg":  "ogg",
	"audio/mp4":  "m4a",
	"audio/mpeg": "mp3",
	"audio/wav":  "wav",
}

// Storage — то немногое, что voice нужно от файлового хранилища: временно
// положить аудио, чтобы Python мог его скачать по ссылке
// (BACKEND_PLAN.md §3.1), и убрать сразу после — хранить его дальше незачем
// (zan-backend-tz-v2.md §4.7).
type Storage interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	PresignGetInternal(ctx context.Context, key string, expiry time.Duration) (string, error)
	Delete(ctx context.Context, key string) error
}

// Transcriber — порт распознавания речи в helper/ через gRPC, реализуется
// internal/grpcclient (BACKEND_PLAN.md §3.3).
type Transcriber interface {
	TranscribeAudio(ctx context.Context, fileURL, mimeType string, lang domain.Language) (string, error)
}

const internalPresignTTL = 5 * time.Minute

// Service — бизнес-логика голосового ввода.
type Service struct {
	storage      Storage
	transcriber  Transcriber
	clock        clock.Clock
	idgen        idgen.Generator
	maxSizeBytes int64
}

// New собирает Service с внедрёнными зависимостями. maxSizeBytes —
// config.Config.VoiceMaxSizeBytes.
func New(storage Storage, transcriber Transcriber, clk clock.Clock, ids idgen.Generator, maxSizeBytes int64) *Service {
	return &Service{storage: storage, transcriber: transcriber, clock: clk, idgen: ids, maxSizeBytes: maxSizeBytes}
}

// TranscribeRequest — вход POST /voice/transcribe.
type TranscribeRequest struct {
	SessionID string
	MimeType  string
	Lang      domain.Language
	Data      []byte
}

// Transcribe — синхронный вызов STT (zan-backend-tz-v2.md §4.7): возвращает
// текст до отправки сообщения в тред, ничего не сохраняет в БД. Стадии
// `stt_started`/`stt_completed` (backend-roadmap.md §6.2, ряд 6).
func (s *Service) Transcribe(ctx context.Context, req TranscribeRequest) (string, error) {
	l := logger.FromContext(ctx)

	ext, ok := AllowedMimeTypes[req.MimeType]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnsupportedMimeType, req.MimeType)
	}
	if int64(len(req.Data)) > s.maxSizeBytes {
		return "", fmt.Errorf("%w: %d bytes (max %d)", ErrTooLarge, len(req.Data), s.maxSizeBytes)
	}

	key := fmt.Sprintf("uploads/%s/%s.%s", req.SessionID, s.idgen.NewID(), ext)
	if err := s.storage.Put(ctx, key, bytes.NewReader(req.Data), int64(len(req.Data)), req.MimeType); err != nil {
		return "", fmt.Errorf("voice: transcribe: put object: %w", err)
	}
	defer s.cleanup(ctx, l, key)

	fileURL, err := s.storage.PresignGetInternal(ctx, key, internalPresignTTL)
	if err != nil {
		return "", fmt.Errorf("voice: transcribe: presign: %w", err)
	}

	l.Info("stt_started", slog.Group("context", slog.Int64("size_bytes", int64(len(req.Data)))))
	started := s.clock.Now()

	text, err := s.transcriber.TranscribeAudio(ctx, fileURL, req.MimeType, req.Lang)
	latencyMs := s.clock.Now().Sub(started).Milliseconds()
	if err != nil {
		l.Warn("stt_completed", slog.Group("context",
			slog.Int64("latency_ms", latencyMs),
			slog.String("warnings", err.Error()),
		))
		return "", err
	}

	l.Info("stt_completed", slog.Group("context", slog.Int64("latency_ms", latencyMs)))

	if text == "" {
		return "", ErrEmptyTranscript
	}
	return text, nil
}

// cleanup — лучшая попытка удалить временный аудиофайл сразу после
// транскрипции (zan-backend-tz-v2.md §4.7: хранить его дальше незачем).
// Неуспех не должен ронять уже полученный результат — только лог.
func (s *Service) cleanup(ctx context.Context, l *slog.Logger, key string) {
	if err := s.storage.Delete(ctx, key); err != nil {
		l.Error("voice_temp_file_cleanup_failed", slog.Group("context",
			slog.String("object_key", key), slog.String("error", err.Error())))
	}
}
