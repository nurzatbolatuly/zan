// Package repo — реализация репозиториев поверх pgx напрямую (без ORM/
// codegen), имплементит интерфейсы, объявленные в internal/service/*
// (BACKEND_CODING_STANDARDS.md §1.1). SQL — обычные строковые литералы
// внутри метода, который их выполняет, не в отдельных .sql-файлах и не
// генерируется: каждый запрос виден целиком там же, где используется.
package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"zan-backend/internal/domain"
	"zan-backend/internal/service/session"
)

// SessionRepo — реализация session.Repository.
type SessionRepo struct {
	db *pgxpool.Pool
}

// NewSessionRepo строит SessionRepo поверх общего пула соединений,
// созданного один раз в composition root (cmd/api) — BACKEND_CODING_STANDARDS.md §2.
func NewSessionRepo(db *pgxpool.Pool) *SessionRepo {
	return &SessionRepo{db: db}
}

func (r *SessionRepo) Create(ctx context.Context, s domain.Session) error {
	id, err := parseUUID(s.ID)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO core.sessions (id, language, onboarding_seen, created_at, last_seen_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, string(s.Language), s.OnboardingSeen, toTimestamptz(s.CreatedAt), toTimestamptz(s.LastSeenAt), toTimestamptz(s.ExpiresAt))
	if err != nil {
		return fmt.Errorf("repo: create session: %w", err)
	}
	return nil
}

func (r *SessionRepo) GetByID(ctx context.Context, id string) (domain.Session, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return domain.Session{}, err
	}

	row := r.db.QueryRow(ctx, `
		SELECT id, language, theme, onboarding_seen, created_at, last_seen_at, expires_at
		FROM core.sessions
		WHERE id = $1
	`, pgID)

	var sr sessionRow
	if err := row.Scan(&sr.id, &sr.language, &sr.theme, &sr.onboardingSeen, &sr.createdAt, &sr.lastSeenAt, &sr.expiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Session{}, session.ErrNotFound
		}
		return domain.Session{}, fmt.Errorf("repo: get session: %w", err)
	}
	return sr.toDomain()
}

func (r *SessionRepo) Update(ctx context.Context, s domain.Session) error {
	id, err := parseUUID(s.ID)
	if err != nil {
		return err
	}

	var theme pgtype.Text
	if s.Theme != nil {
		theme = pgtype.Text{String: string(*s.Theme), Valid: true}
	}

	_, err = r.db.Exec(ctx, `
		UPDATE core.sessions
		SET language        = $2,
		    theme           = $3,
		    onboarding_seen = $4,
		    last_seen_at    = $5,
		    expires_at      = $6
		WHERE id = $1
	`, id, string(s.Language), theme, s.OnboardingSeen, toTimestamptz(s.LastSeenAt), toTimestamptz(s.ExpiresAt))
	if err != nil {
		return fmt.Errorf("repo: update session: %w", err)
	}
	return nil
}

// sessionRow — форма строки core.sessions под Scan; не запрос, чистое
// хранилище отсканированных колонок на пути к domain.Session. Отдельный тип
// вместо N позиционных параметров в конвертере — безопаснее против ошибки в
// порядке аргументов при росте числа колонок.
type sessionRow struct {
	id             pgtype.UUID
	language       string
	theme          pgtype.Text
	onboardingSeen bool
	createdAt      pgtype.Timestamptz
	lastSeenAt     pgtype.Timestamptz
	expiresAt      pgtype.Timestamptz
}

func (sr sessionRow) toDomain() (domain.Session, error) {
	lang, err := domain.ParseLanguage(sr.language)
	if err != nil {
		// CHECK-ограничение в БД (000002_core_schema.up.sql) уже не даёт
		// записать невалидное значение — это невозможное состояние,
		// сигнал о рассинхроне схемы/domain, не штатная ошибка запроса.
		return domain.Session{}, fmt.Errorf("repo: session row: %w", err)
	}

	var theme *domain.Theme
	if sr.theme.Valid {
		t, err := domain.ParseTheme(sr.theme.String)
		if err != nil {
			return domain.Session{}, fmt.Errorf("repo: session row: %w", err)
		}
		theme = &t
	}

	return domain.Session{
		ID:             fromPgUUID(sr.id),
		Language:       lang,
		Theme:          theme,
		OnboardingSeen: sr.onboardingSeen,
		CreatedAt:      sr.createdAt.Time,
		LastSeenAt:     sr.lastSeenAt.Time,
		ExpiresAt:      sr.expiresAt.Time,
	}, nil
}
