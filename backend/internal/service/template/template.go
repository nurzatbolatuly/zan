// Package template — шаблоны документов и справочник их типов
// (/admin/document-types, /admin/document-templates). Шаблон — загруженный
// админом образец документа (PDF/DOCX), отнесённый к типу из справочника;
// для просмотра в админке у каждого шаблона есть PDF-копия (у DOCX её
// строит helper/ — порт Converter). Порты объявлены здесь, где используются
// (BACKEND_CODING_STANDARDS.md §1.1).
package template

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/platform/logger"
)

const (
	// MaxTypeNameLength/MaxTitleLength — в символах, не байтах: подписи
	// карточек в админке, не тексты.
	MaxTypeNameLength = 100
	MaxTitleLength    = 200

	mimePDF  = "application/pdf"
	mimeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

	// internalPresignTTL — ссылка для helper/ на время одного синхронного
	// вызова ConvertToPdf (тот же порядок величины, что у file.Service).
	internalPresignTTL = 5 * time.Minute
	// previewPresignTTL — ссылка на PDF-копию для браузера; список шаблонов
	// перечитывается при каждом открытии вкладки, ссылка выпускается заново.
	previewPresignTTL = 24 * time.Hour
)

var (
	// ErrTypeNotFound — нет типа документа с таким id.
	ErrTypeNotFound = errors.New("template: document type not found")
	// ErrTypeNameTaken — тип с таким именем (без учёта регистра) уже есть.
	ErrTypeNameTaken = errors.New("template: document type name already taken")
	// ErrTypeInUse — у типа есть шаблоны; сначала их нужно удалить или
	// перенести в другой тип (молча удалять чужие файлы вместе с типом нельзя).
	ErrTypeInUse = errors.New("template: document type has templates")
	// ErrNotFound — нет шаблона с таким id.
	ErrNotFound = errors.New("template: not found")
	// ErrInvalidInput — пустое/слишком длинное имя типа или название шаблона,
	// отсутствует файл.
	ErrInvalidInput = errors.New("template: invalid input")
	ErrTooLarge     = errors.New("template: file too large")
	// ErrUnsupportedFormat — шаблоном может быть только PDF или DOCX.
	ErrUnsupportedFormat = errors.New("template: unsupported file format")
	// ErrConversionFailed — helper/ не смог построить PDF-копию DOCX
	// (повреждённый файл или helper/ недоступен); шаблон не сохраняется.
	ErrConversionFailed = errors.New("template: pdf conversion failed")
)

// Repository — порт доступа к core.document_types/core.document_templates.
// Ошибки — сентинелы этого пакета: ErrTypeNameTaken (уникальность имени),
// ErrTypeInUse (удаление типа с шаблонами), ErrTypeNotFound/ErrNotFound.
type Repository interface {
	ListTypes(ctx context.Context) ([]domain.DocumentType, error)
	GetType(ctx context.Context, id string) (domain.DocumentType, error)
	CreateType(ctx context.Context, t domain.DocumentType) (domain.DocumentType, error)
	RenameType(ctx context.Context, id, name string, updatedAt time.Time) (domain.DocumentType, error)
	DeleteType(ctx context.Context, id string) error

	ListTemplates(ctx context.Context) ([]domain.DocumentTemplate, error)
	GetTemplate(ctx context.Context, id string) (domain.DocumentTemplate, error)
	CreateTemplate(ctx context.Context, t domain.DocumentTemplate) (domain.DocumentTemplate, error)
	// UpdateTemplate — полная замена изменяемых полей (тип, название, файл).
	UpdateTemplate(ctx context.Context, t domain.DocumentTemplate) (domain.DocumentTemplate, error)
	DeleteTemplate(ctx context.Context, id string) error
}

// Storage — порт S3-совместимого хранилища (internal/platform/storage).
type Storage interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Delete(ctx context.Context, key string) error
	PresignGetPublic(ctx context.Context, key string, expiry time.Duration) (string, error)
	PresignGetInternal(ctx context.Context, key string, expiry time.Duration) (string, error)
}

// Converter — порт DOCX -> PDF в helper/ (internal/grpcclient,
// FilesService.ConvertToPdf). Возвращает ключ готового PDF в том же бакете.
type Converter interface {
	ConvertToPDF(ctx context.Context, fileURL, mimeType string) (string, error)
}

// Service — бизнес-логика шаблонов документов.
type Service struct {
	repo         Repository
	storage      Storage
	converter    Converter
	clock        clock.Clock
	idgen        idgen.Generator
	maxSizeBytes int64
}

// New собирает Service. maxSizeBytes — тот же config.Config.FileMaxSizeBytes,
// что у вложений чата.
func New(repo Repository, storage Storage, converter Converter, clk clock.Clock, ids idgen.Generator, maxSizeBytes int64) *Service {
	return &Service{
		repo:         repo,
		storage:      storage,
		converter:    converter,
		clock:        clk,
		idgen:        ids,
		maxSizeBytes: maxSizeBytes,
	}
}

// File — загружаемый файл шаблона. Тип определяется по расширению имени и
// сигнатуре содержимого, Content-Type от клиента не используется: браузеры
// нередко присылают для DOCX application/octet-stream.
type File struct {
	OriginalName string
	Data         []byte
}

// CreateRequest — POST /admin/document-templates.
type CreateRequest struct {
	DocumentTypeID string
	Title          string
	File           File
}

// UpdateRequest — PUT /admin/document-templates/{id}. File == nil — файл
// остаётся прежним, меняются только тип и название.
type UpdateRequest struct {
	DocumentTypeID string
	Title          string
	File           *File
}

// TemplateView — шаблон со свежей presigned-ссылкой на его PDF-копию.
type TemplateView struct {
	Template   domain.DocumentTemplate
	PreviewURL string
}

// --- Типы документов ---

// ListTypes — GET /admin/document-types, по алфавиту.
func (s *Service) ListTypes(ctx context.Context) ([]domain.DocumentType, error) {
	types, err := s.repo.ListTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("template: list types: %w", err)
	}
	return types, nil
}

// CreateType — POST /admin/document-types.
func (s *Service) CreateType(ctx context.Context, name string) (domain.DocumentType, error) {
	name, err := normalizeText(name, "name", MaxTypeNameLength)
	if err != nil {
		return domain.DocumentType{}, err
	}
	now := s.clock.Now()
	created, err := s.repo.CreateType(ctx, domain.DocumentType{ID: s.idgen.NewID(), Name: name, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return domain.DocumentType{}, fmt.Errorf("template: create type: %w", err)
	}
	logger.FromContext(ctx).Info("document_type_created", slog.Group("context", slog.String("document_type_id", created.ID)))
	return created, nil
}

// RenameType — PUT /admin/document-types/{id}.
func (s *Service) RenameType(ctx context.Context, id, name string) (domain.DocumentType, error) {
	name, err := normalizeText(name, "name", MaxTypeNameLength)
	if err != nil {
		return domain.DocumentType{}, err
	}
	updated, err := s.repo.RenameType(ctx, id, name, s.clock.Now())
	if err != nil {
		return domain.DocumentType{}, fmt.Errorf("template: rename type: %w", err)
	}
	logger.FromContext(ctx).Info("document_type_renamed", slog.Group("context", slog.String("document_type_id", id)))
	return updated, nil
}

// DeleteType — DELETE /admin/document-types/{id}; ErrTypeInUse, если у типа
// есть шаблоны.
func (s *Service) DeleteType(ctx context.Context, id string) error {
	if err := s.repo.DeleteType(ctx, id); err != nil {
		return fmt.Errorf("template: delete type: %w", err)
	}
	logger.FromContext(ctx).Info("document_type_deleted", slog.Group("context", slog.String("document_type_id", id)))
	return nil
}

// --- Шаблоны ---

// List — GET /admin/document-templates, новые сверху.
func (s *Service) List(ctx context.Context) ([]TemplateView, error) {
	templates, err := s.repo.ListTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("template: list: %w", err)
	}
	views := make([]TemplateView, 0, len(templates))
	for _, t := range templates {
		view, err := s.view(ctx, t)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// Create — POST /admin/document-templates: сохраняет файл и его PDF-копию,
// затем запись. Неудача после загрузки в хранилище убирает уже загруженные
// объекты — шаблон либо создан целиком, либо не оставляет следов.
func (s *Service) Create(ctx context.Context, req CreateRequest) (TemplateView, error) {
	title, err := s.validateFields(ctx, req.DocumentTypeID, req.Title)
	if err != nil {
		return TemplateView{}, err
	}
	stored, err := s.storeFile(ctx, req.File)
	if err != nil {
		return TemplateView{}, err
	}

	now := s.clock.Now()
	created, err := s.repo.CreateTemplate(ctx, domain.DocumentTemplate{
		ID:               s.idgen.NewID(),
		DocumentTypeID:   req.DocumentTypeID,
		Title:            title,
		ObjectKey:        stored.ObjectKey,
		OriginalName:     stored.OriginalName,
		MimeType:         stored.MimeType,
		SizeBytes:        stored.SizeBytes,
		PreviewObjectKey: stored.PreviewObjectKey,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		s.deleteObjects(ctx, stored.objectKeys())
		return TemplateView{}, fmt.Errorf("template: create: %w", err)
	}
	logger.FromContext(ctx).Info("document_template_created", slog.Group("context",
		slog.String("template_id", created.ID),
		slog.String("document_type_id", created.DocumentTypeID),
		slog.String("mime_type", created.MimeType),
		slog.Int64("size_bytes", created.SizeBytes),
	))
	return s.view(ctx, created)
}

// Update — PUT /admin/document-templates/{id}. При замене файла старые
// объекты удаляются только после успешного сохранения записи.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (TemplateView, error) {
	existing, err := s.repo.GetTemplate(ctx, id)
	if err != nil {
		return TemplateView{}, err
	}
	title, err := s.validateFields(ctx, req.DocumentTypeID, req.Title)
	if err != nil {
		return TemplateView{}, err
	}

	next := existing
	next.DocumentTypeID = req.DocumentTypeID
	next.Title = title
	next.UpdatedAt = s.clock.Now()
	if req.File != nil {
		stored, err := s.storeFile(ctx, *req.File)
		if err != nil {
			return TemplateView{}, err
		}
		next.ObjectKey = stored.ObjectKey
		next.OriginalName = stored.OriginalName
		next.MimeType = stored.MimeType
		next.SizeBytes = stored.SizeBytes
		next.PreviewObjectKey = stored.PreviewObjectKey
	}

	updated, err := s.repo.UpdateTemplate(ctx, next)
	if err != nil {
		if req.File != nil {
			s.deleteObjects(ctx, next.ObjectKeys())
		}
		return TemplateView{}, fmt.Errorf("template: update: %w", err)
	}
	if req.File != nil {
		s.deleteObjects(ctx, existing.ObjectKeys())
	}
	logger.FromContext(ctx).Info("document_template_updated", slog.Group("context",
		slog.String("template_id", id),
		slog.Bool("file_replaced", req.File != nil),
	))
	return s.view(ctx, updated)
}

// Delete — DELETE /admin/document-templates/{id}: запись, затем объекты
// хранилища (их неудачное удаление не возвращает шаблон — только WARN в лог).
func (s *Service) Delete(ctx context.Context, id string) error {
	existing, err := s.repo.GetTemplate(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteTemplate(ctx, id); err != nil {
		return fmt.Errorf("template: delete: %w", err)
	}
	s.deleteObjects(ctx, existing.ObjectKeys())
	logger.FromContext(ctx).Info("document_template_deleted", slog.Group("context", slog.String("template_id", id)))
	return nil
}

// validateFields — общая проверка тела Create/Update: название и
// существующий тип. Тип проверяется до загрузки файла, чтобы не тратить
// хранилище и конвертацию на заведомо отклонённый запрос.
func (s *Service) validateFields(ctx context.Context, documentTypeID, title string) (string, error) {
	title, err := normalizeText(title, "title", MaxTitleLength)
	if err != nil {
		return "", err
	}
	if _, err := s.repo.GetType(ctx, documentTypeID); err != nil {
		return "", err
	}
	return title, nil
}

// storedFile — файл шаблона, уже лежащий в хранилище вместе с PDF-копией.
type storedFile struct {
	ObjectKey        string
	PreviewObjectKey string
	OriginalName     string
	MimeType         string
	SizeBytes        int64
}

func (f storedFile) objectKeys() []string {
	return domain.DocumentTemplate{ObjectKey: f.ObjectKey, PreviewObjectKey: f.PreviewObjectKey}.ObjectKeys()
}

func (s *Service) storeFile(ctx context.Context, f File) (storedFile, error) {
	if len(f.Data) == 0 {
		return storedFile{}, fmt.Errorf("%w: file is required", ErrInvalidInput)
	}
	if int64(len(f.Data)) > s.maxSizeBytes {
		return storedFile{}, fmt.Errorf("%w: %d bytes (max %d)", ErrTooLarge, len(f.Data), s.maxSizeBytes)
	}
	mimeType, err := detectMimeType(f)
	if err != nil {
		return storedFile{}, err
	}

	// Ключ без расширения — тот же принцип, что у вложений (file.Service):
	// тип объекта задаёт Content-Type, не суффикс ключа.
	key := "templates/" + s.idgen.NewID()
	if err := s.storage.Put(ctx, key, bytes.NewReader(f.Data), int64(len(f.Data)), mimeType); err != nil {
		return storedFile{}, fmt.Errorf("template: put object: %w", err)
	}

	stored := storedFile{
		ObjectKey:        key,
		PreviewObjectKey: key,
		OriginalName:     f.OriginalName,
		MimeType:         mimeType,
		SizeBytes:        int64(len(f.Data)),
	}
	if mimeType == mimePDF {
		return stored, nil
	}

	previewKey, err := s.convertToPDF(ctx, key, mimeType)
	if err != nil {
		s.deleteObjects(ctx, []string{key})
		return storedFile{}, err
	}
	stored.PreviewObjectKey = previewKey
	return stored, nil
}

func (s *Service) convertToPDF(ctx context.Context, key, mimeType string) (string, error) {
	fileURL, err := s.storage.PresignGetInternal(ctx, key, internalPresignTTL)
	if err != nil {
		return "", fmt.Errorf("template: presign for conversion: %w", err)
	}
	previewKey, err := s.converter.ConvertToPDF(ctx, fileURL, mimeType)
	if err != nil {
		logger.FromContext(ctx).Warn("document_template_conversion_failed", slog.Group("context",
			slog.String("object_key", key),
			slog.String("error", err.Error()),
		))
		return "", fmt.Errorf("%w: %w", ErrConversionFailed, err)
	}
	return previewKey, nil
}

// deleteObjects — best-effort: осиротевший объект в бакете не стоит
// провала операции, которую пользователь уже видит успешной.
func (s *Service) deleteObjects(ctx context.Context, keys []string) {
	for _, key := range keys {
		if err := s.storage.Delete(ctx, key); err != nil {
			logger.FromContext(ctx).Warn("document_template_object_delete_failed", slog.Group("context",
				slog.String("object_key", key),
				slog.String("error", err.Error()),
			))
		}
	}
}

func (s *Service) view(ctx context.Context, t domain.DocumentTemplate) (TemplateView, error) {
	url, err := s.storage.PresignGetPublic(ctx, t.PreviewObjectKey, previewPresignTTL)
	if err != nil {
		return TemplateView{}, fmt.Errorf("template: presign preview: %w", err)
	}
	return TemplateView{Template: t, PreviewURL: url}, nil
}

// detectMimeType — PDF/DOCX по расширению имени, подтверждённому сигнатурой
// содержимого (%PDF- / zip-контейнер): переименованный файл другого типа
// не должен дойти до конвертера.
func detectMimeType(f File) (string, error) {
	ext := strings.ToLower(filepath.Ext(f.OriginalName))
	switch {
	case ext == ".pdf" && bytes.HasPrefix(f.Data, []byte("%PDF-")):
		return mimePDF, nil
	case ext == ".docx" && bytes.HasPrefix(f.Data, []byte("PK\x03\x04")):
		return mimeDOCX, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedFormat, f.OriginalName)
	}
}

func normalizeText(value, field string, maxLength int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: %s must not be empty", ErrInvalidInput, field)
	}
	if utf8.RuneCountInString(value) > maxLength {
		return "", fmt.Errorf("%w: %s must be at most %d characters", ErrInvalidInput, field, maxLength)
	}
	return value, nil
}
