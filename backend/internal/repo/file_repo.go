package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"zan-backend/internal/domain"
	"zan-backend/internal/service/file"
)

// FileRepo — реализация file.Repository поверх pgx напрямую (тот же стиль,
// что остальной internal/repo — SQL как строковый литерал в методе, который
// его выполняет, без codegen).
type FileRepo struct {
	db *pgxpool.Pool
}

// NewFileRepo строит FileRepo поверх общего пула соединений.
func NewFileRepo(db *pgxpool.Pool) *FileRepo {
	return &FileRepo{db: db}
}

func (r *FileRepo) Create(ctx context.Context, f domain.FileAttachment) (domain.FileAttachment, error) {
	id, err := parseUUID(f.ID)
	if err != nil {
		return domain.FileAttachment{}, err
	}
	sessionID, err := parseUUID(f.SessionID)
	if err != nil {
		return domain.FileAttachment{}, err
	}
	outputFormatsJSON, err := marshalIfNotEmpty(f.OutputFormats)
	if err != nil {
		return domain.FileAttachment{}, fmt.Errorf("repo: marshal output formats: %w", err)
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO core.file_attachments (
		    id, session_id, message_id, thread_id, object_key, original_name, mime_type, size_bytes,
		    purpose, output_formats, processing_status, extracted_text, created_at
		)
		VALUES ($1, $2, NULL, NULL, $3, $4, $5, $6, $7, $8, $9, NULL, $10)
		RETURNING id, session_id, message_id, thread_id, object_key, original_name, mime_type, size_bytes,
		          purpose, output_formats, processing_status, extracted_text, created_at
	`, id, sessionID, f.ObjectKey, f.OriginalName, f.MimeType, f.SizeBytes,
		string(f.Purpose), outputFormatsJSON, string(f.ProcessingStatus), toTimestamptz(f.CreatedAt))

	var fr fileRow
	if err := fr.scan(row); err != nil {
		return domain.FileAttachment{}, fmt.Errorf("repo: create file attachment: %w", err)
	}
	return fr.toDomain()
}

func (r *FileRepo) GetByID(ctx context.Context, id string) (domain.FileAttachment, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return domain.FileAttachment{}, file.ErrNotFound
	}

	row := r.db.QueryRow(ctx, `
		SELECT id, session_id, message_id, thread_id, object_key, original_name, mime_type, size_bytes,
		       purpose, output_formats, processing_status, extracted_text, created_at
		FROM core.file_attachments
		WHERE id = $1
	`, pgID)

	var fr fileRow
	if err := fr.scan(row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.FileAttachment{}, file.ErrNotFound
		}
		return domain.FileAttachment{}, fmt.Errorf("repo: get file attachment: %w", err)
	}
	return fr.toDomain()
}

func (r *FileRepo) SetProcessingResult(ctx context.Context, id string, status domain.FileProcessingStatus, extractedText *string) error {
	pgID, err := parseUUID(id)
	if err != nil {
		return err
	}
	var text pgtype.Text
	if extractedText != nil {
		text = pgtype.Text{String: *extractedText, Valid: true}
	}

	_, err = r.db.Exec(ctx, `
		UPDATE core.file_attachments
		SET processing_status = $2,
		    extracted_text    = $3
		WHERE id = $1
	`, pgID, string(status), text)
	if err != nil {
		return fmt.Errorf("repo: set file processing result: %w", err)
	}
	return nil
}

// ValidateAvailable — см. file.Repository: не различает наружу "не
// существует"/"чужая сессия"/"уже привязан" — все три дают file.ErrNotFound
// (BACKEND_CODING_STANDARDS.md — не подтверждать существование чужого id).
func (r *FileRepo) ValidateAvailable(ctx context.Context, sessionID string, ids []string) error {
	pgSessionID, err := parseUUID(sessionID)
	if err != nil {
		return err
	}

	row := r.db.QueryRow(ctx, `
		SELECT count(*)
		FROM core.file_attachments
		WHERE id = ANY($1::uuid[])
		  AND session_id = $2
		  AND message_id IS NULL
	`, ids, pgSessionID)

	var matched int
	if err := row.Scan(&matched); err != nil {
		return fmt.Errorf("repo: validate available files: %w", err)
	}
	if matched != len(ids) {
		return file.ErrNotFound
	}
	return nil
}

func (r *FileRepo) AttachToMessage(ctx context.Context, messageID string, ids []string) error {
	pgMessageID, err := parseUUID(messageID)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(ctx, `
		UPDATE core.file_attachments
		SET message_id = $1
		WHERE id = ANY($2::uuid[])
	`, pgMessageID, ids)
	if err != nil {
		return fmt.Errorf("repo: attach files to message: %w", err)
	}
	return nil
}

// CreateGenerated — вставляет FileAttachment(purpose=generated_output)
// созданный самим backend'ом (Stage 6: POST /threads/{id}/generate-document),
// не загрузкой — отдельный метод от Create, не общий: message_id/thread_id
// заданы сразу (не NULL, как при Create — файл существует только ПОСЛЕ того,
// как ассистентский Message уже сохранён, см. internal/service/document),
// свой INSERT-литерал (BACKEND_CODING_STANDARDS.md §1.1 — разные по смыслу
// операции не делят общий запрос).
func (r *FileRepo) CreateGenerated(ctx context.Context, f domain.FileAttachment) (domain.FileAttachment, error) {
	id, err := parseUUID(f.ID)
	if err != nil {
		return domain.FileAttachment{}, err
	}
	sessionID, err := parseUUID(f.SessionID)
	if err != nil {
		return domain.FileAttachment{}, err
	}
	if f.MessageID == nil || f.ThreadID == nil {
		return domain.FileAttachment{}, fmt.Errorf("repo: create generated file: message_id and thread_id are required")
	}
	messageID, err := parseUUID(*f.MessageID)
	if err != nil {
		return domain.FileAttachment{}, err
	}
	threadID, err := parseUUID(*f.ThreadID)
	if err != nil {
		return domain.FileAttachment{}, err
	}
	outputFormatsJSON, err := marshalIfNotEmpty(f.OutputFormats)
	if err != nil {
		return domain.FileAttachment{}, fmt.Errorf("repo: marshal output formats: %w", err)
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO core.file_attachments (
		    id, session_id, message_id, thread_id, object_key, original_name, mime_type, size_bytes,
		    purpose, output_formats, processing_status, extracted_text, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NULL, $12)
		RETURNING id, session_id, message_id, thread_id, object_key, original_name, mime_type, size_bytes,
		          purpose, output_formats, processing_status, extracted_text, created_at
	`, id, sessionID, messageID, threadID, f.ObjectKey, f.OriginalName, f.MimeType, f.SizeBytes,
		string(f.Purpose), outputFormatsJSON, string(f.ProcessingStatus), toTimestamptz(f.CreatedAt))

	var fr fileRow
	if err := fr.scan(row); err != nil {
		return domain.FileAttachment{}, fmt.Errorf("repo: create generated file: %w", err)
	}
	return fr.toDomain()
}

// GetLatestGenerated — последний сгенерированный файл треда в данном
// формате (Stage 6: GET /threads/{id}/document?format=) — по mime_type, не
// по формату из запроса напрямую (document.Service переводит один в другой,
// FileRepo не знает про grpcclient.RenderFormat, BACKEND_CODING_STANDARDS.md
// §1.1). "Последний" — на случай повторной генерации (document.Service
// каждый вызов POST /threads/{id}/generate-document создаёт новую пару
// файлов, не переиспользует старую).
func (r *FileRepo) GetLatestGenerated(ctx context.Context, threadID, mimeType string) (domain.FileAttachment, error) {
	pgThreadID, err := parseUUID(threadID)
	if err != nil {
		return domain.FileAttachment{}, file.ErrNotFound
	}

	row := r.db.QueryRow(ctx, `
		SELECT id, session_id, message_id, thread_id, object_key, original_name, mime_type, size_bytes,
		       purpose, output_formats, processing_status, extracted_text, created_at
		FROM core.file_attachments
		WHERE thread_id = $1
		  AND mime_type = $2
		  AND purpose = 'generated_output'
		ORDER BY created_at DESC
		LIMIT 1
	`, pgThreadID, mimeType)

	var fr fileRow
	if err := fr.scan(row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.FileAttachment{}, file.ErrNotFound
		}
		return domain.FileAttachment{}, fmt.Errorf("repo: get latest generated file: %w", err)
	}
	return fr.toDomain()
}

// fileRow — форма строки core.file_attachments под Scan (тот же приём, что
// sessionRow/threadRow — отдельный тип вместо позиционных параметров).
type fileRow struct {
	id               pgtype.UUID
	sessionID        pgtype.UUID
	messageID        pgtype.UUID
	threadID         pgtype.UUID
	objectKey        string
	originalName     string
	mimeType         string
	sizeBytes        int64
	purpose          string
	outputFormats    []byte
	processingStatus string
	extractedText    pgtype.Text
	createdAt        pgtype.Timestamptz
}

func (fr *fileRow) scan(row pgx.Row) error {
	return row.Scan(
		&fr.id, &fr.sessionID, &fr.messageID, &fr.threadID, &fr.objectKey, &fr.originalName, &fr.mimeType, &fr.sizeBytes,
		&fr.purpose, &fr.outputFormats, &fr.processingStatus, &fr.extractedText, &fr.createdAt,
	)
}

func (fr fileRow) toDomain() (domain.FileAttachment, error) {
	purpose, err := domain.ParseFilePurpose(fr.purpose)
	if err != nil {
		return domain.FileAttachment{}, fmt.Errorf("repo: file row: %w", err)
	}
	status, err := domain.ParseFileProcessingStatus(fr.processingStatus)
	if err != nil {
		return domain.FileAttachment{}, fmt.Errorf("repo: file row: %w", err)
	}

	var messageID *string
	if fr.messageID.Valid {
		v := fromPgUUID(fr.messageID)
		messageID = &v
	}
	var threadID *string
	if fr.threadID.Valid {
		v := fromPgUUID(fr.threadID)
		threadID = &v
	}

	var outputFormats []string
	if len(fr.outputFormats) > 0 {
		if err := json.Unmarshal(fr.outputFormats, &outputFormats); err != nil {
			return domain.FileAttachment{}, fmt.Errorf("repo: file row: unmarshal output formats: %w", err)
		}
	}

	var extractedText *string
	if fr.extractedText.Valid {
		extractedText = &fr.extractedText.String
	}

	return domain.FileAttachment{
		ID:               fromPgUUID(fr.id),
		SessionID:        fromPgUUID(fr.sessionID),
		MessageID:        messageID,
		ThreadID:         threadID,
		ObjectKey:        fr.objectKey,
		OriginalName:     fr.originalName,
		MimeType:         fr.mimeType,
		SizeBytes:        fr.sizeBytes,
		Purpose:          purpose,
		OutputFormats:    outputFormats,
		ProcessingStatus: status,
		ExtractedText:    extractedText,
		CreatedAt:        fr.createdAt.Time,
	}, nil
}
