// Package file — юзкейсы загрузки/хранения файлов, прикреплённых к Thread
// (zan-backend-tz-v2.md §5). Оркестрирует хранилище метаданных, S3 и
// извлечение текста в helper/ через gRPC.
package file

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
	// ErrNotFound — файла с таким id нет, либо он принадлежит другой сессии
	// (тот же принцип, что thread.ErrThreadNotFound: не подтверждать
	// существование чужого id, BACKEND_CODING_STANDARDS.md).
	ErrNotFound = errors.New("file: not found")

	// ErrUnsupportedMimeType — zan-backend-tz-v3.md §5.3.
	ErrUnsupportedMimeType = errors.New("file: unsupported mime type")

	// ErrTooLarge — zan-backend-tz-v3.md §5.3.
	ErrTooLarge = errors.New("file: too large")

	// ErrRejectedByAVScanner — zan-backend-tz-v3.md §5.3: "файл не
	// сохраняется, событие логируется как security-инцидент".
	ErrRejectedByAVScanner = errors.New("file: rejected by antivirus scan")
)

// AllowedMimeTypes — зафиксировано на Stage 4 (BACKEND_PLAN.md §6 п.13):
// ровно то, что нужно для анализа документов в MVP (zan-backend-tz-v3.md
// §5.3) — значение маппится на расширение object key, не расширение из
// имени файла (original_name — произвольная строка от клиента, доверять её
// расширению для маршрутизации Extract нельзя).
var AllowedMimeTypes = map[string]string{
	"application/pdf": "pdf",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "docx",
	"image/png":  "png",
	"image/jpeg": "jpg",
}

// internalPresignTTL — время жизни presigned-ссылки, которую видит только
// helper/ (синхронный вызов в пределах одного HTTP-запроса, секунд более
// чем достаточно).
const internalPresignTTL = 5 * time.Minute

// publicPresignTTL — время жизни ссылки, которую видит браузер клиента
// (POST /files/upload, GET /files/{id}) — сутки: не одноразовая, но и не
// бессрочная (GET /files/{id} перевыпускает свежую при каждом обращении,
// см. domain.FileAttachment).
const publicPresignTTL = 24 * time.Hour

// Repository — порт доступа к метаданным FileAttachment.
type Repository interface {
	Create(ctx context.Context, f domain.FileAttachment) (domain.FileAttachment, error)
	GetByID(ctx context.Context, id string) (domain.FileAttachment, error)
	SetProcessingResult(ctx context.Context, id string, status domain.FileProcessingStatus, extractedText *string) error

	// ValidateAvailable — все id существуют, принадлежат sessionID и ещё не
	// прикреплены ни к одному сообщению (message_id IS NULL). Возвращает
	// ErrNotFound на первое несовпадение, не различая наружу "не
	// существует"/"чужая сессия"/"уже привязан" — тот же принцип, что
	// billing.GetPayment (не подтверждать существование чужого id).
	ValidateAvailable(ctx context.Context, sessionID string, ids []string) error

	// AttachToMessage — проставляет message_id уже провалидированным
	// (ValidateAvailable) набору id. Отдельный шаг, не общая транзакция с
	// созданием сообщения — тот же принятый компромисс, что
	// billing.ConfirmPayment -> thread.ActivateAfterPayment (Stage 3):
	// изолированный неуспех здесь не должен откатывать уже созданное
	// сообщение.
	AttachToMessage(ctx context.Context, messageID string, ids []string) error
}

// Storage — порт S3-совместимого файлового хранилища, реализуется
// internal/platform/storage.
type Storage interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	PresignGetPublic(ctx context.Context, key string, expiry time.Duration) (string, error)
	PresignGetInternal(ctx context.Context, key string, expiry time.Duration) (string, error)
}

// Extractor — порт извлечения текста в helper/ через gRPC, реализуется
// internal/grpcclient (BACKEND_PLAN.md §3.3).
type Extractor interface {
	ExtractFile(ctx context.Context, fileURL, mimeType string) (string, error)
}

// AVScanner — антивирусная проверка загруженного файла
// (zan-backend-tz-v3.md §5.3). Порт объявлен как Strategy, тот же приём,
// что SttProvider на стороне Python — composition root (cmd/api/main.go)
// подключает internal/platform/clamav.Scanner (Stage 8, clamd поверх
// TCP), сам Service от конкретной реализации не зависит.
type AVScanner interface {
	// Scan возвращает ErrRejectedByAVScanner, если данные признаны опасными.
	Scan(ctx context.Context, data []byte) error
}

// Service — бизнес-логика файлов.
type Service struct {
	repo         Repository
	storage      Storage
	extractor    Extractor
	avScanner    AVScanner
	clock        clock.Clock
	idgen        idgen.Generator
	maxSizeBytes int64
}

// New собирает Service с внедрёнными зависимостями. maxSizeBytes —
// config.Config.FileMaxSizeBytes (BACKEND_PLAN.md §6 п.13).
func New(repo Repository, storage Storage, extractor Extractor, avScanner AVScanner, clk clock.Clock, ids idgen.Generator, maxSizeBytes int64) *Service {
	return &Service{
		repo:         repo,
		storage:      storage,
		extractor:    extractor,
		avScanner:    avScanner,
		clock:        clk,
		idgen:        ids,
		maxSizeBytes: maxSizeBytes,
	}
}

// UploadRequest — вход POST /files/upload.
type UploadRequest struct {
	SessionID    string
	OriginalName string
	MimeType     string
	Data         []byte
}

// Upload сохраняет файл в S3 и синхронно вызывает FilesService.Extract
// (BACKEND_PLAN.md Stage 4 DoD: "реальный PDF/DOCX -> извлечённый текст...
// через Go-оркестрацию и gRPC-вызовы, на одном запросе"). Неудача Extract —
// не фатальна для аплоада (zan-backend-tz-v3.md §5.3: "файл всё равно
// сохраняется, processing_status=error"), фатальны только MIME/размер/AV.
// Возвращает вместе с записью свежую presigned-ссылку на скачивание
// (zan-backend-tz-v2.md §3.3: "{file_id, url}").
func (s *Service) Upload(ctx context.Context, req UploadRequest) (domain.FileAttachment, string, error) {
	l := logger.FromContext(ctx)

	ext, ok := AllowedMimeTypes[req.MimeType]
	if !ok {
		return domain.FileAttachment{}, "", fmt.Errorf("%w: %q", ErrUnsupportedMimeType, req.MimeType)
	}
	if int64(len(req.Data)) > s.maxSizeBytes {
		return domain.FileAttachment{}, "", fmt.Errorf("%w: %d bytes (max %d)", ErrTooLarge, len(req.Data), s.maxSizeBytes)
	}

	if err := s.avScanner.Scan(ctx, req.Data); err != nil {
		l.Log(ctx, logger.LevelCritical, "file_av_scan_rejected", slog.Group("context",
			slog.String("session_id", req.SessionID),
			slog.String("original_name", req.OriginalName),
		))
		return domain.FileAttachment{}, "", ErrRejectedByAVScanner
	}

	id := s.idgen.NewID()
	key := fmt.Sprintf("uploads/%s/%s.%s", req.SessionID, id, ext)
	if err := s.storage.Put(ctx, key, bytes.NewReader(req.Data), int64(len(req.Data)), req.MimeType); err != nil {
		return domain.FileAttachment{}, "", fmt.Errorf("file: upload: put object: %w", err)
	}

	created, err := s.repo.Create(ctx, domain.FileAttachment{
		ID:               id,
		SessionID:        req.SessionID,
		ObjectKey:        key,
		OriginalName:     req.OriginalName,
		MimeType:         req.MimeType,
		SizeBytes:        int64(len(req.Data)),
		Purpose:          domain.FilePurposeAnalysisInput,
		ProcessingStatus: domain.FileProcessingStatusPending,
		CreatedAt:        s.clock.Now(),
	})
	if err != nil {
		return domain.FileAttachment{}, "", fmt.Errorf("file: upload: create record: %w", err)
	}
	l.Info("file_uploaded", slog.Group("context",
		slog.String("file_id", created.ID),
		slog.String("session_id", req.SessionID),
		slog.String("mime_type", req.MimeType),
		slog.Int64("size_bytes", created.SizeBytes),
	))

	created = s.extract(ctx, l, created)

	url, err := s.storage.PresignGetPublic(ctx, created.ObjectKey, publicPresignTTL)
	if err != nil {
		return domain.FileAttachment{}, "", fmt.Errorf("file: upload: presign: %w", err)
	}
	return created, url, nil
}

// extract — FilesService.Extract, стадии `file_extract_started`/
// `file_extract_completed` (backend-roadmap.md §6.2, ряд 5).
func (s *Service) extract(ctx context.Context, l *slog.Logger, f domain.FileAttachment) domain.FileAttachment {
	l.Info("file_extract_started", slog.Group("context", slog.String("file_id", f.ID)))
	started := s.clock.Now()

	fail := func(reason string) domain.FileAttachment {
		latencyMs := s.clock.Now().Sub(started).Milliseconds()
		l.Warn("file_extract_completed", slog.Group("context",
			slog.String("file_id", f.ID),
			slog.Int("extracted_chars", 0),
			slog.String("warnings", reason),
			slog.Int64("latency_ms", latencyMs),
		))
		if err := s.repo.SetProcessingResult(ctx, f.ID, domain.FileProcessingStatusError, nil); err != nil {
			l.Error("file_extract_status_update_failed", slog.Group("context",
				slog.String("file_id", f.ID), slog.String("error", err.Error())))
		}
		f.ProcessingStatus = domain.FileProcessingStatusError
		return f
	}

	fileURL, err := s.storage.PresignGetInternal(ctx, f.ObjectKey, internalPresignTTL)
	if err != nil {
		return fail(fmt.Sprintf("presign failed: %s", err))
	}

	text, err := s.extractor.ExtractFile(ctx, fileURL, f.MimeType)
	if err != nil {
		return fail(err.Error())
	}

	latencyMs := s.clock.Now().Sub(started).Milliseconds()
	l.Info("file_extract_completed", slog.Group("context",
		slog.String("file_id", f.ID),
		slog.Int("extracted_chars", len(text)),
		slog.Int64("latency_ms", latencyMs),
	))
	if err := s.repo.SetProcessingResult(ctx, f.ID, domain.FileProcessingStatusProcessed, &text); err != nil {
		l.Error("file_extract_status_update_failed", slog.Group("context",
			slog.String("file_id", f.ID), slog.String("error", err.Error())))
	}
	f.ProcessingStatus = domain.FileProcessingStatusProcessed
	f.ExtractedText = &text
	return f
}

// GetByID — GET /files/{id}: метаданные + свежая presigned-ссылка на
// скачивание (zan-backend-tz-v2.md §3.3: "метаданные/скачивание").
// sessionID — владелец, запрошенный не своей сессией file_id трактуется
// как ErrNotFound (BACKEND_CODING_STANDARDS.md — не подтверждать
// существование чужого id).
func (s *Service) GetByID(ctx context.Context, sessionID, id string) (domain.FileAttachment, string, error) {
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.FileAttachment{}, "", err
	}
	if f.SessionID != sessionID {
		return domain.FileAttachment{}, "", ErrNotFound
	}
	url, err := s.storage.PresignGetPublic(ctx, f.ObjectKey, publicPresignTTL)
	if err != nil {
		return domain.FileAttachment{}, "", fmt.Errorf("file: get: presign: %w", err)
	}
	return f, url, nil
}

// ValidateAvailable — вызывается thread.Service перед созданием сообщения
// с input_type=file (zan-backend-tz-v2.md §3.2 "file_ids?"): все fileIDs
// существуют, принадлежат sessionID, ещё не привязаны.
func (s *Service) ValidateAvailable(ctx context.Context, sessionID string, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	return s.repo.ValidateAvailable(ctx, sessionID, fileIDs)
}

// AttachToMessage — вызывается thread.Service после успешного создания
// сообщения (см. ValidateAvailable — компромисс "изолированный неуспех не
// откатывает уже созданное сообщение", логируется вызывающим кодом).
func (s *Service) AttachToMessage(ctx context.Context, messageID string, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	return s.repo.AttachToMessage(ctx, messageID, fileIDs)
}
