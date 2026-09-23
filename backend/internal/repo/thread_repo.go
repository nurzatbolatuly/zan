package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"zan-backend/internal/domain"
	"zan-backend/internal/service/thread"
)

// ThreadRepo — реализация thread.Repository (Stage 3). CreateThread/
// AppendMessage открывают явную транзакцию (тот же приём, что
// BillingRepo.ConfirmPayment/DebitCredit — BACKEND_CODING_STANDARDS.md
// §1.1): вставка Thread+Message или Message+счётчики треда должны либо
// применяться вместе, либо не применяться совсем. Переходы статус-машины
// (UpdateStatus) — одной guarded UPDATE, транзакция не нужна.
type ThreadRepo struct {
	db *pgxpool.Pool
}

// NewThreadRepo строит ThreadRepo поверх общего пула соединений.
func NewThreadRepo(db *pgxpool.Pool) *ThreadRepo {
	return &ThreadRepo{db: db}
}

func (r *ThreadRepo) CreateThread(ctx context.Context, t domain.Thread, firstMessage domain.Message) (domain.Thread, domain.Message, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Thread{}, domain.Message{}, fmt.Errorf("repo: begin create thread: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := parseUUID(t.ID)
	if err != nil {
		return domain.Thread{}, domain.Message{}, err
	}
	sessionID, err := parseUUID(t.SessionID)
	if err != nil {
		return domain.Thread{}, domain.Message{}, err
	}
	messageCount, err := toInt32(t.MessageCount)
	if err != nil {
		return domain.Thread{}, domain.Message{}, err
	}

	threadRowResult := tx.QueryRow(ctx, `
		INSERT INTO core.threads (
		    id, session_id, service_id, status, title, preview_text, message_count,
		    created_at, last_message_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, session_id, service_id, status, title, preview_text, message_count, created_at, last_message_at, deleted_at
	`, id, sessionID, t.ServiceID, string(t.Status), t.Title, t.PreviewText, messageCount,
		toTimestamptz(t.CreatedAt), optionalTimestamptz(t.LastMessageAt))

	var tr threadRow
	if err := tr.scan(threadRowResult); err != nil {
		return domain.Thread{}, domain.Message{}, fmt.Errorf("repo: create thread: %w", err)
	}

	msgRowResult, err := insertMessage(ctx, tx, firstMessage, tr.id)
	if err != nil {
		return domain.Thread{}, domain.Message{}, fmt.Errorf("repo: create thread: insert first message: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Thread{}, domain.Message{}, fmt.Errorf("repo: commit create thread: %w", err)
	}

	created, err := tr.toDomain()
	if err != nil {
		return domain.Thread{}, domain.Message{}, err
	}
	msg, err := msgRowResult.toDomain()
	return created, msg, err
}

func (r *ThreadRepo) GetByID(ctx context.Context, id string) (domain.Thread, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return domain.Thread{}, thread.ErrThreadNotFound
	}

	row := r.db.QueryRow(ctx, `
		SELECT id, session_id, service_id, status, title, preview_text, message_count, created_at, last_message_at, deleted_at
		FROM core.threads
		WHERE id = $1
	`, pgID)

	var tr threadRow
	if err := tr.scan(row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Thread{}, thread.ErrThreadNotFound
		}
		return domain.Thread{}, fmt.Errorf("repo: get thread: %w", err)
	}
	return tr.toDomain()
}

func (r *ThreadRepo) GetMessages(ctx context.Context, threadID string) ([]domain.Message, error) {
	pgThreadID, err := parseUUID(threadID)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, thread_id, sender, input_type, text, sources, findings, feedback, processing_time_ms, created_at
		FROM core.messages
		WHERE thread_id = $1
		ORDER BY created_at ASC
	`, pgThreadID)
	if err != nil {
		return nil, fmt.Errorf("repo: get messages: %w", err)
	}
	defer rows.Close()

	var msgs []domain.Message
	for rows.Next() {
		var mr messageRow
		if err := mr.scan(rows); err != nil {
			return nil, fmt.Errorf("repo: get messages: scan: %w", err)
		}
		msg, err := mr.toDomain()
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: get messages: %w", err)
	}
	rows.Close()

	// Вложения — вторым запросом, а не JOIN'ом: у сообщения может быть
	// несколько файлов, строки сообщений не должны размножаться.
	// extracted_text не читается (NULL) — см. domain.Message.Attachments.
	fileRows, err := r.db.Query(ctx, `
		SELECT f.id, f.session_id, f.message_id, f.thread_id, f.object_key, f.original_name, f.mime_type, f.size_bytes,
		       f.purpose, f.output_formats, f.processing_status, NULL::text AS extracted_text, f.created_at
		FROM core.file_attachments f
		JOIN core.messages m ON m.id = f.message_id
		WHERE m.thread_id = $1
		  AND f.purpose = 'analysis_input'
		ORDER BY f.created_at ASC
	`, pgThreadID)
	if err != nil {
		return nil, fmt.Errorf("repo: get messages: attachments: %w", err)
	}
	defer fileRows.Close()

	byMessageID := make(map[string][]domain.FileAttachment)
	for fileRows.Next() {
		var fr fileRow
		if err := fr.scan(fileRows); err != nil {
			return nil, fmt.Errorf("repo: get messages: attachments: scan: %w", err)
		}
		f, err := fr.toDomain()
		if err != nil {
			return nil, err
		}
		byMessageID[*f.MessageID] = append(byMessageID[*f.MessageID], f)
	}
	if err := fileRows.Err(); err != nil {
		return nil, fmt.Errorf("repo: get messages: attachments: %w", err)
	}
	for i := range msgs {
		msgs[i].Attachments = byMessageID[msgs[i].ID]
	}
	return msgs, nil
}

// GetConversation — история треда для LLM: GetMessages (сообщения с
// метаданными вложений) плюс извлечённый текст каждого вложения.
func (r *ThreadRepo) GetConversation(ctx context.Context, threadID string) ([]domain.Message, error) {
	msgs, err := r.GetMessages(ctx, threadID)
	if err != nil {
		return nil, err
	}
	pgThreadID, err := parseUUID(threadID)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT f.id, f.extracted_text
		FROM core.file_attachments f
		JOIN core.messages m ON m.id = f.message_id
		WHERE m.thread_id = $1
		  AND f.purpose = 'analysis_input'
		  AND f.extracted_text IS NOT NULL
	`, pgThreadID)
	if err != nil {
		return nil, fmt.Errorf("repo: get conversation: extracted text: %w", err)
	}
	defer rows.Close()

	textByFileID := make(map[string]string)
	for rows.Next() {
		var (
			id   pgtype.UUID
			text string
		)
		if err := rows.Scan(&id, &text); err != nil {
			return nil, fmt.Errorf("repo: get conversation: extracted text: scan: %w", err)
		}
		textByFileID[fromPgUUID(id)] = text
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: get conversation: extracted text: %w", err)
	}

	for i := range msgs {
		for j := range msgs[i].Attachments {
			if text, ok := textByFileID[msgs[i].Attachments[j].ID]; ok {
				msgs[i].Attachments[j].ExtractedText = &text
			}
		}
	}
	return msgs, nil
}

func (r *ThreadRepo) ListThreads(ctx context.Context, sessionID string, filter thread.ListFilter) ([]domain.Thread, int, error) {
	pgSessionID, err := parseUUID(sessionID)
	if err != nil {
		return nil, 0, err
	}
	var status pgtype.Text
	if filter.Status != nil {
		status = pgtype.Text{String: string(*filter.Status), Valid: true}
	}
	var search pgtype.Text
	if filter.Search != "" {
		search = pgtype.Text{String: filter.Search, Valid: true}
	}
	pageSize, err := toInt32(filter.PageSize)
	if err != nil {
		return nil, 0, err
	}
	offset, err := toInt32((filter.Page - 1) * filter.PageSize)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT threads.id, threads.session_id, threads.service_id, threads.status, threads.title, threads.preview_text, threads.message_count, threads.created_at, threads.last_message_at, threads.deleted_at, count(*) OVER () AS total_count
		FROM core.threads AS threads
		WHERE session_id = $1
		  AND deleted_at IS NULL
		  AND ($2::text IS NULL OR status = $2::text)
		  AND (
		    $3::text IS NULL
		    OR title ILIKE '%' || $3::text || '%'
		    OR preview_text ILIKE '%' || $3::text || '%'
		  )
		ORDER BY created_at DESC
		LIMIT $5 OFFSET $4
	`, pgSessionID, status, search, offset, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: list threads: %w", err)
	}
	defer rows.Close()

	var items []domain.Thread
	total := 0
	first := true
	for rows.Next() {
		var tr threadRow
		var totalCount int64
		if err := rows.Scan(
			&tr.id, &tr.sessionID, &tr.serviceID, &tr.status, &tr.title, &tr.previewText, &tr.messageCount,
			&tr.createdAt, &tr.lastMessageAt, &tr.deletedAt,
			&totalCount,
		); err != nil {
			return nil, 0, fmt.Errorf("repo: list threads: scan: %w", err)
		}
		t, err := tr.toDomain()
		if err != nil {
			return nil, 0, err
		}
		items = append(items, t)
		if first {
			total = int(totalCount)
			first = false
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo: list threads: %w", err)
	}
	return items, total, nil
}

// AppendMessage — вставляет сообщение и атомарно увеличивает
// message_count/last_message_at владельца треда одной транзакцией.
func (r *ThreadRepo) AppendMessage(ctx context.Context, msg domain.Message) (domain.Thread, error) {
	pgThreadID, err := parseUUID(msg.ThreadID)
	if err != nil {
		return domain.Thread{}, err
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Thread{}, fmt.Errorf("repo: begin append message: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := insertMessage(ctx, tx, msg, pgThreadID); err != nil {
		return domain.Thread{}, fmt.Errorf("repo: append message: insert: %w", err)
	}

	row := tx.QueryRow(ctx, `
		UPDATE core.threads
		SET message_count   = message_count + 1,
		    last_message_at = $2
		WHERE id = $1
		RETURNING id, session_id, service_id, status, title, preview_text, message_count, created_at, last_message_at, deleted_at
	`, pgThreadID, toTimestamptz(msg.CreatedAt))

	var tr threadRow
	if err := tr.scan(row); err != nil {
		return domain.Thread{}, fmt.Errorf("repo: append message: touch thread: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Thread{}, fmt.Errorf("repo: commit append message: %w", err)
	}
	return tr.toDomain()
}

func (r *ThreadRepo) UpdateStatus(ctx context.Context, id string, from []domain.ThreadStatus, to domain.ThreadStatus, previewText string) (domain.Thread, bool, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return domain.Thread{}, false, err
	}
	fromStatuses := make([]string, len(from))
	for i, s := range from {
		fromStatuses[i] = string(s)
	}

	// Гарантированный переход status: WHERE status = ANY(from) — тот же
	// паттерн, что MarkPaymentSuccess в billing_repo.go (0 строк = статус
	// уже не из набора from, не ошибка, вызывающий Go-код это отличает по
	// pgx.ErrNoRows).
	row := r.db.QueryRow(ctx, `
		UPDATE core.threads
		SET status       = $2,
		    preview_text = $3
		WHERE id = $1
		  AND status = ANY($4::text[])
		RETURNING id, session_id, service_id, status, title, preview_text, message_count, created_at, last_message_at, deleted_at
	`, pgID, string(to), previewText, fromStatuses)

	var tr threadRow
	if err := tr.scan(row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Thread{}, false, nil
		}
		return domain.Thread{}, false, fmt.Errorf("repo: update thread status: %w", err)
	}
	t, err := tr.toDomain()
	return t, true, err
}

func (r *ThreadRepo) SoftDelete(ctx context.Context, id string, now time.Time) (bool, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return false, err
	}

	row := r.db.QueryRow(ctx, `
		UPDATE core.threads
		SET deleted_at = $2
		WHERE id = $1
		  AND deleted_at IS NULL
		RETURNING id
	`, pgID, toTimestamptz(now))

	var discardedID pgtype.UUID
	if err := row.Scan(&discardedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("repo: soft delete thread: %w", err)
	}
	return true, nil
}

// SetMessageFeedback — thread_id IN (...) вместо отдельного SELECT на
// ownership: один запрос, 0 строк одинаково значит "сообщения нет" и "чужая
// сессия" (не палим факт существования чужого id, тот же принцип, что
// billing.GetPaymentByID).
func (r *ThreadRepo) SetMessageFeedback(ctx context.Context, messageID, sessionID string, feedback domain.MessageFeedback) (domain.Message, error) {
	pgMessageID, err := parseUUID(messageID)
	if err != nil {
		return domain.Message{}, thread.ErrMessageNotFound
	}
	pgSessionID, err := parseUUID(sessionID)
	if err != nil {
		return domain.Message{}, thread.ErrMessageNotFound
	}

	row := r.db.QueryRow(ctx, `
		UPDATE core.messages AS m
		SET feedback = $2
		WHERE m.id = $1
		  AND m.thread_id IN (SELECT t.id FROM core.threads AS t WHERE t.session_id = $3)
		RETURNING m.id, m.thread_id, m.sender, m.input_type, m.text, m.sources, m.findings, m.feedback, m.processing_time_ms, m.created_at
	`, pgMessageID, pgtype.Text{String: string(feedback), Valid: true}, pgSessionID)

	var mr messageRow
	if err := mr.scan(row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Message{}, thread.ErrMessageNotFound
		}
		return domain.Message{}, fmt.Errorf("repo: set message feedback: %w", err)
	}
	return mr.toDomain()
}

// insertMessage — общий INSERT для CreateThread/AppendMessage: оба
// открывают свою транзакцию и вставляют ровно одно и то же сообщение,
// разница только в наборе сопутствующих операций внутри той же tx
// (создание треда vs инкремент счётчиков). db — tx.QueryRow-совместимый
// хендл (pgx.Tx), не *pgxpool.Pool: вызывается только внутри транзакции.
func insertMessage(ctx context.Context, tx pgx.Tx, m domain.Message, pgThreadID pgtype.UUID) (messageRow, error) {
	id, err := parseUUID(m.ID)
	if err != nil {
		return messageRow{}, err
	}
	sourcesJSON, err := marshalIfNotEmpty(m.Sources)
	if err != nil {
		return messageRow{}, fmt.Errorf("marshal message sources: %w", err)
	}
	findingsJSON, err := marshalIfNotEmpty(m.Findings)
	if err != nil {
		return messageRow{}, fmt.Errorf("marshal message findings: %w", err)
	}
	processingTimeMs, err := optionalInt32(m.ProcessingTimeMs)
	if err != nil {
		return messageRow{}, err
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO core.messages (
		    id, thread_id, sender, input_type, text, sources, findings, processing_time_ms, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, thread_id, sender, input_type, text, sources, findings, feedback, processing_time_ms, created_at
	`, id, pgThreadID, string(m.Sender), string(m.InputType), m.Text, sourcesJSON, findingsJSON, processingTimeMs, toTimestamptz(m.CreatedAt))

	var mr messageRow
	if err := mr.scan(row); err != nil {
		return messageRow{}, err
	}
	return mr, nil
}

// marshalIfNotEmpty — nil (не пустой JSON-массив "[]") для пустых
// срезов, чтобы nullable jsonb-колонка получила SQL NULL, а не
// физическое значение "[]"/"null" (pgx трактует nil []byte как NULL-параметр).
func marshalIfNotEmpty[T any](v []T) ([]byte, error) {
	if len(v) == 0 {
		return nil, nil
	}
	return json.Marshal(v)
}

// threadRow/messageRow — формы строк core.threads/core.messages под Scan;
// не запросы, чистое хранилище отсканированных колонок (см. sessionRow).
// scan() принимает и pgx.Row (QueryRow), и *pgx.Rows (Query — реализует ту
// же Scan-сигнатуру) через общий интерфейс rowScanner.

type rowScanner interface {
	Scan(dest ...any) error
}

type threadRow struct {
	id            pgtype.UUID
	sessionID     pgtype.UUID
	serviceID     string
	status        string
	title         string
	previewText   string
	messageCount  int32
	createdAt     pgtype.Timestamptz
	lastMessageAt pgtype.Timestamptz
	deletedAt     pgtype.Timestamptz
}

func (tr *threadRow) scan(row rowScanner) error {
	return row.Scan(
		&tr.id, &tr.sessionID, &tr.serviceID, &tr.status, &tr.title, &tr.previewText, &tr.messageCount,
		&tr.createdAt, &tr.lastMessageAt, &tr.deletedAt,
	)
}

func (tr threadRow) toDomain() (domain.Thread, error) {
	status, err := domain.ParseThreadStatus(tr.status)
	if err != nil {
		return domain.Thread{}, fmt.Errorf("repo: thread row: %w", err)
	}
	return domain.Thread{
		ID:            fromPgUUID(tr.id),
		SessionID:     fromPgUUID(tr.sessionID),
		ServiceID:     tr.serviceID,
		Status:        status,
		Title:         tr.title,
		PreviewText:   tr.previewText,
		MessageCount:  int(tr.messageCount),
		CreatedAt:     tr.createdAt.Time,
		LastMessageAt: fromOptionalTimestamptz(tr.lastMessageAt),
		DeletedAt:     fromOptionalTimestamptz(tr.deletedAt),
	}, nil
}

type messageRow struct {
	id               pgtype.UUID
	threadID         pgtype.UUID
	sender           string
	inputType        string
	text             string
	sources          []byte
	findings         []byte
	feedback         pgtype.Text
	processingTimeMs pgtype.Int4
	createdAt        pgtype.Timestamptz
}

func (mr *messageRow) scan(row rowScanner) error {
	return row.Scan(
		&mr.id, &mr.threadID, &mr.sender, &mr.inputType, &mr.text,
		&mr.sources, &mr.findings, &mr.feedback, &mr.processingTimeMs, &mr.createdAt,
	)
}

func (mr messageRow) toDomain() (domain.Message, error) {
	sender, err := domain.ParseMessageSender(mr.sender)
	if err != nil {
		return domain.Message{}, fmt.Errorf("repo: message row: %w", err)
	}
	inputType, err := domain.ParseMessageInputType(mr.inputType)
	if err != nil {
		return domain.Message{}, fmt.Errorf("repo: message row: %w", err)
	}

	var sources []domain.Source
	if len(mr.sources) > 0 {
		if err := json.Unmarshal(mr.sources, &sources); err != nil {
			return domain.Message{}, fmt.Errorf("repo: message row: unmarshal sources: %w", err)
		}
	}
	var findings []domain.Finding
	if len(mr.findings) > 0 {
		if err := json.Unmarshal(mr.findings, &findings); err != nil {
			return domain.Message{}, fmt.Errorf("repo: message row: unmarshal findings: %w", err)
		}
	}

	var feedback *domain.MessageFeedback
	if mr.feedback.Valid {
		f, err := domain.ParseMessageFeedback(mr.feedback.String)
		if err != nil {
			return domain.Message{}, fmt.Errorf("repo: message row: %w", err)
		}
		feedback = &f
	}
	var processingTimeMs *int
	if mr.processingTimeMs.Valid {
		v := int(mr.processingTimeMs.Int32)
		processingTimeMs = &v
	}

	return domain.Message{
		ID:               fromPgUUID(mr.id),
		ThreadID:         fromPgUUID(mr.threadID),
		Sender:           sender,
		InputType:        inputType,
		Text:             mr.text,
		Sources:          sources,
		Findings:         findings,
		Feedback:         feedback,
		ProcessingTimeMs: processingTimeMs,
		CreatedAt:        mr.createdAt.Time,
	}, nil
}

func optionalTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return toTimestamptz(*t)
}

func fromOptionalTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// optionalInt32 — *int -> pgtype.Int4, nil даёт SQL NULL (тот же приём,
// что optionalTimestamptz выше). Stage 7: Message.ProcessingTimeMs — nil,
// пока агент не ответил (сообщение пользователя), заполнен для ответа
// ассистента (internal/service/thread.finishWithAnswer).
func optionalInt32(v *int) (pgtype.Int4, error) {
	if v == nil {
		return pgtype.Int4{}, nil
	}
	n, err := toInt32(*v)
	if err != nil {
		return pgtype.Int4{}, err
	}
	return pgtype.Int4{Int32: n, Valid: true}, nil
}
