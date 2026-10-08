package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"zan-backend/internal/domain"
	"zan-backend/internal/service/template"
)

// Коды ошибок Postgres, которые этот репозиторий переводит в сентинелы
// template: уникальность имени типа и FK шаблон -> тип. Проверка ложится на
// БД, а не на предварительный SELECT — без гонки между проверкой и записью.
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
)

// TemplateRepo — реализация template.Repository поверх
// core.document_types/core.document_templates (миграция 000011).
type TemplateRepo struct {
	db *pgxpool.Pool
}

// NewTemplateRepo строит TemplateRepo поверх общего пула соединений.
func NewTemplateRepo(db *pgxpool.Pool) *TemplateRepo {
	return &TemplateRepo{db: db}
}

func (r *TemplateRepo) ListTypes(ctx context.Context) ([]domain.DocumentType, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, created_at, updated_at
		FROM core.document_types
		ORDER BY lower(name)
	`)
	if err != nil {
		return nil, fmt.Errorf("repo: list document types: %w", err)
	}
	defer rows.Close()

	types := []domain.DocumentType{}
	for rows.Next() {
		var tr documentTypeRow
		if err := tr.scan(rows); err != nil {
			return nil, fmt.Errorf("repo: list document types: scan: %w", err)
		}
		types = append(types, tr.toDomain())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: list document types: %w", err)
	}
	return types, nil
}

func (r *TemplateRepo) GetType(ctx context.Context, id string) (domain.DocumentType, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return domain.DocumentType{}, template.ErrTypeNotFound
	}

	row := r.db.QueryRow(ctx, `
		SELECT id, name, created_at, updated_at
		FROM core.document_types
		WHERE id = $1
	`, pgID)

	var tr documentTypeRow
	if err := tr.scan(row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DocumentType{}, template.ErrTypeNotFound
		}
		return domain.DocumentType{}, fmt.Errorf("repo: get document type: %w", err)
	}
	return tr.toDomain(), nil
}

func (r *TemplateRepo) CreateType(ctx context.Context, t domain.DocumentType) (domain.DocumentType, error) {
	pgID, err := parseUUID(t.ID)
	if err != nil {
		return domain.DocumentType{}, err
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO core.document_types (id, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, created_at, updated_at
	`, pgID, t.Name, toTimestamptz(t.CreatedAt), toTimestamptz(t.UpdatedAt))

	var tr documentTypeRow
	if err := tr.scan(row); err != nil {
		if isPgError(err, pgUniqueViolation) {
			return domain.DocumentType{}, template.ErrTypeNameTaken
		}
		return domain.DocumentType{}, fmt.Errorf("repo: create document type: %w", err)
	}
	return tr.toDomain(), nil
}

func (r *TemplateRepo) RenameType(ctx context.Context, id, name string, updatedAt time.Time) (domain.DocumentType, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return domain.DocumentType{}, template.ErrTypeNotFound
	}

	row := r.db.QueryRow(ctx, `
		UPDATE core.document_types
		SET name = $2,
		    updated_at = $3
		WHERE id = $1
		RETURNING id, name, created_at, updated_at
	`, pgID, name, toTimestamptz(updatedAt))

	var tr documentTypeRow
	if err := tr.scan(row); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return domain.DocumentType{}, template.ErrTypeNotFound
		case isPgError(err, pgUniqueViolation):
			return domain.DocumentType{}, template.ErrTypeNameTaken
		default:
			return domain.DocumentType{}, fmt.Errorf("repo: rename document type: %w", err)
		}
	}
	return tr.toDomain(), nil
}

func (r *TemplateRepo) DeleteType(ctx context.Context, id string) error {
	pgID, err := parseUUID(id)
	if err != nil {
		return template.ErrTypeNotFound
	}

	tag, err := r.db.Exec(ctx, `
		DELETE FROM core.document_types
		WHERE id = $1
	`, pgID)
	if err != nil {
		if isPgError(err, pgForeignKeyViolation) {
			return template.ErrTypeInUse
		}
		return fmt.Errorf("repo: delete document type: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return template.ErrTypeNotFound
	}
	return nil
}

func (r *TemplateRepo) ListTemplates(ctx context.Context) ([]domain.DocumentTemplate, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, document_type_id, title, object_key, original_name, mime_type, size_bytes,
		       preview_object_key, created_at, updated_at
		FROM core.document_templates
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("repo: list document templates: %w", err)
	}
	defer rows.Close()

	templates := []domain.DocumentTemplate{}
	for rows.Next() {
		var tr documentTemplateRow
		if err := tr.scan(rows); err != nil {
			return nil, fmt.Errorf("repo: list document templates: scan: %w", err)
		}
		templates = append(templates, tr.toDomain())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: list document templates: %w", err)
	}
	return templates, nil
}

func (r *TemplateRepo) GetTemplate(ctx context.Context, id string) (domain.DocumentTemplate, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return domain.DocumentTemplate{}, template.ErrNotFound
	}

	row := r.db.QueryRow(ctx, `
		SELECT id, document_type_id, title, object_key, original_name, mime_type, size_bytes,
		       preview_object_key, created_at, updated_at
		FROM core.document_templates
		WHERE id = $1
	`, pgID)

	var tr documentTemplateRow
	if err := tr.scan(row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DocumentTemplate{}, template.ErrNotFound
		}
		return domain.DocumentTemplate{}, fmt.Errorf("repo: get document template: %w", err)
	}
	return tr.toDomain(), nil
}

func (r *TemplateRepo) CreateTemplate(ctx context.Context, t domain.DocumentTemplate) (domain.DocumentTemplate, error) {
	pgID, err := parseUUID(t.ID)
	if err != nil {
		return domain.DocumentTemplate{}, err
	}
	typeID, err := parseUUID(t.DocumentTypeID)
	if err != nil {
		return domain.DocumentTemplate{}, template.ErrTypeNotFound
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO core.document_templates (
		    id, document_type_id, title, object_key, original_name, mime_type, size_bytes,
		    preview_object_key, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, document_type_id, title, object_key, original_name, mime_type, size_bytes,
		          preview_object_key, created_at, updated_at
	`, pgID, typeID, t.Title, t.ObjectKey, t.OriginalName, t.MimeType, t.SizeBytes,
		t.PreviewObjectKey, toTimestamptz(t.CreatedAt), toTimestamptz(t.UpdatedAt))

	var tr documentTemplateRow
	if err := tr.scan(row); err != nil {
		if isPgError(err, pgForeignKeyViolation) {
			return domain.DocumentTemplate{}, template.ErrTypeNotFound
		}
		return domain.DocumentTemplate{}, fmt.Errorf("repo: create document template: %w", err)
	}
	return tr.toDomain(), nil
}

func (r *TemplateRepo) UpdateTemplate(ctx context.Context, t domain.DocumentTemplate) (domain.DocumentTemplate, error) {
	pgID, err := parseUUID(t.ID)
	if err != nil {
		return domain.DocumentTemplate{}, template.ErrNotFound
	}
	typeID, err := parseUUID(t.DocumentTypeID)
	if err != nil {
		return domain.DocumentTemplate{}, template.ErrTypeNotFound
	}

	row := r.db.QueryRow(ctx, `
		UPDATE core.document_templates
		SET document_type_id   = $2,
		    title              = $3,
		    object_key         = $4,
		    original_name      = $5,
		    mime_type          = $6,
		    size_bytes         = $7,
		    preview_object_key = $8,
		    updated_at         = $9
		WHERE id = $1
		RETURNING id, document_type_id, title, object_key, original_name, mime_type, size_bytes,
		          preview_object_key, created_at, updated_at
	`, pgID, typeID, t.Title, t.ObjectKey, t.OriginalName, t.MimeType, t.SizeBytes,
		t.PreviewObjectKey, toTimestamptz(t.UpdatedAt))

	var tr documentTemplateRow
	if err := tr.scan(row); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return domain.DocumentTemplate{}, template.ErrNotFound
		case isPgError(err, pgForeignKeyViolation):
			return domain.DocumentTemplate{}, template.ErrTypeNotFound
		default:
			return domain.DocumentTemplate{}, fmt.Errorf("repo: update document template: %w", err)
		}
	}
	return tr.toDomain(), nil
}

func (r *TemplateRepo) DeleteTemplate(ctx context.Context, id string) error {
	pgID, err := parseUUID(id)
	if err != nil {
		return template.ErrNotFound
	}

	tag, err := r.db.Exec(ctx, `
		DELETE FROM core.document_templates
		WHERE id = $1
	`, pgID)
	if err != nil {
		return fmt.Errorf("repo: delete document template: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return template.ErrNotFound
	}
	return nil
}

func isPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

type documentTypeRow struct {
	id        pgtype.UUID
	name      string
	createdAt pgtype.Timestamptz
	updatedAt pgtype.Timestamptz
}

func (tr *documentTypeRow) scan(row rowScanner) error {
	return row.Scan(&tr.id, &tr.name, &tr.createdAt, &tr.updatedAt)
}

func (tr documentTypeRow) toDomain() domain.DocumentType {
	return domain.DocumentType{
		ID:        fromPgUUID(tr.id),
		Name:      tr.name,
		CreatedAt: tr.createdAt.Time,
		UpdatedAt: tr.updatedAt.Time,
	}
}

type documentTemplateRow struct {
	id               pgtype.UUID
	documentTypeID   pgtype.UUID
	title            string
	objectKey        string
	originalName     string
	mimeType         string
	sizeBytes        int64
	previewObjectKey string
	createdAt        pgtype.Timestamptz
	updatedAt        pgtype.Timestamptz
}

func (tr *documentTemplateRow) scan(row rowScanner) error {
	return row.Scan(
		&tr.id, &tr.documentTypeID, &tr.title, &tr.objectKey, &tr.originalName, &tr.mimeType, &tr.sizeBytes,
		&tr.previewObjectKey, &tr.createdAt, &tr.updatedAt,
	)
}

func (tr documentTemplateRow) toDomain() domain.DocumentTemplate {
	return domain.DocumentTemplate{
		ID:               fromPgUUID(tr.id),
		DocumentTypeID:   fromPgUUID(tr.documentTypeID),
		Title:            tr.title,
		ObjectKey:        tr.objectKey,
		OriginalName:     tr.originalName,
		MimeType:         tr.mimeType,
		SizeBytes:        tr.sizeBytes,
		PreviewObjectKey: tr.previewObjectKey,
		CreatedAt:        tr.createdAt.Time,
		UpdatedAt:        tr.updatedAt.Time,
	}
}
