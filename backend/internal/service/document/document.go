// Package document — Stage 6 ("Агент 'Документы'", BACKEND_PLAN.md):
// генерация документа по треду (POST /threads/{id}/generate-document) и его
// скачивание (GET /threads/{id}/document?format=). Отдельная операция от
// Q&A-треда (zan-backend-tz-v2.md §4.4: "генерация документа — всегда
// отдельная операция, платная отдельно от типа треда, service_id='doc'"),
// не идёт через internal/service/thread.Service.CreateThread/AddMessage —
// переиспользует уже готовую LLM-инфраструктуру Stage 5 (internal/agent,
// новый метод GenerateDocument) и рендер-пайплайн Stage 4
// (grpcclient.Client.RenderDocument), но со своей оркестрацией: билинг,
// сохранение ответа в чат, рендер PDF/DOCX и хранение результата — всё
// здесь, по тому же принципу, что internal/service/thread оркестрирует
// billing/catalog/file/agent для Q&A (BACKEND_CODING_STANDARDS.md §1.1).
package document

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"zan-backend/internal/domain"
	"zan-backend/internal/grpcclient"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/thread"
)

// serviceID — услуга, которой оплачивается генерация документа
// (zan-backend-tz-v2.md §4.4: "с service_id='doc'"), заведена сидом
// миграции 000003_stage2_billing наравне с "qa" (internal/service/catalog).
const serviceID = "doc"

// publicPresignTTL — тот же срок, что file.Service использует для
// пользовательских ссылок на скачивание (сутки — не одноразовая, не
// бессрочная; свежая ссылка перевыпускается на каждый ответ, см.
// domain.FileAttachment).
const publicPresignTTL = 24 * time.Hour

var (
	// ErrThreadNotFound — реэкспорт thread.ErrThreadNotFound: "треда с таким
	// id нет, либо он принадлежит другой сессии, либо soft-deleted" — то же
	// самое условие и тот же ответ клиенту, что и у internal/service/thread
	// (BACKEND_CODING_STANDARDS.md — не подтверждать существование чужого
	// id), заводить второй сентинел с идентичным смыслом бессмысленно.
	ErrThreadNotFound = thread.ErrThreadNotFound

	// ErrGenerationFailed — LLM не смогла подготовить текст документа
	// (таймаут/невалидный JSON/отказ модели/needs_clarification — всё, что
	// backend-roadmap.md §5.2 трактует как единый исход "не удалось
	// обработать запрос" для Q&A, здесь тот же принцип: генерация документа
	// не вводит отдельного статуса "уточнение", любой неуспех LLM —
	// ErrGenerationFailed). Кредит уже возвращён к моменту, когда эта ошибка
	// доходит до вызывающего кода (см. Service.Generate). В отличие от
	// неуспеха рендера (см. Service.Generate — filesReady=false, не error:
	// здесь ничего не сохранено в тред вообще, полноценная HTTP-ошибка).
	ErrGenerationFailed = errors.New("document: generation failed")

	// ErrNotGenerated — GET /threads/{id}/document?format=: для этого треда
	// ещё ни разу не сгенерирован документ в запрошенном формате.
	ErrNotGenerated = errors.New("document: not generated yet")

	// ErrInvalidFormat — format вне "pdf"/"docx".
	ErrInvalidFormat = errors.New("document: invalid format")
)

// ThreadStore — то немногое, что document нужно от треда: загрузить его
// (владение/статус проверяет сам Service), историю для промпта, сохранить
// ответ ассистента. Порт объявлен здесь, где используется
// (BACKEND_CODING_STANDARDS.md §1.1) — *repo.ThreadRepo (реализация
// thread.Repository, Stage 3) удовлетворяет ему структурно без изменений,
// второй репозиторий не заводится.
type ThreadStore interface {
	GetByID(ctx context.Context, id string) (domain.Thread, error)
	// GetConversation — история треда с вложениями пользователя (тот же
	// метод, что thread.Repository.GetConversation): документ по договору
	// должен опираться на текст самого договора.
	GetConversation(ctx context.Context, threadID string) ([]domain.Message, error)
	// AppendMessage — тот же метод, что thread.Repository.AppendMessage:
	// вставляет сообщение и атомарно обновляет message_count/last_message_at
	// владельца-треда (internal/repo/thread_repo.go) — сгенерированный
	// документ отражается в истории треда тем же способом, что и обычный
	// ответ ассистента.
	AppendMessage(ctx context.Context, msg domain.Message) (domain.Thread, error)
}

// FileStore — доступ к FileAttachment для сгенерированных файлов
// (purpose=generated_output). Порт объявлен здесь; *repo.FileRepo
// удовлетворяет ему структурно (те же два метода, что использует и
// internal/service/file — общая таблица core.file_attachments, разные
// операции над ней, BACKEND_CODING_STANDARDS.md §1.1).
type FileStore interface {
	CreateGenerated(ctx context.Context, f domain.FileAttachment) (domain.FileAttachment, error)
	GetLatestGenerated(ctx context.Context, threadID, mimeType string) (domain.FileAttachment, error)
}

// BalanceService — списание/возврат кредита "doc" (тот же узкий порт, что
// thread.BalanceService, объявлен отдельно — *billing.Service удовлетворяет
// обоим структурно, без импорта пакета thread ради этого,
// BACKEND_CODING_STANDARDS.md §1.1).
type BalanceService interface {
	DebitCredit(ctx context.Context, sessionID, serviceID string) error
	RefundCredit(ctx context.Context, sessionID, serviceID string, qty int) error
}

// ServiceCatalog — проверка, что услуга "doc" существует/активна, прежде
// чем списывать баланс (тот же принцип, что thread.ServiceCatalog).
type ServiceCatalog interface {
	GetServiceByID(ctx context.Context, id string) (domain.Service, error)
}

// GenerateResult — структурированный результат генерации документа
// (answer_text/sources/findings), без Status — у генерации документа нет
// отдельного "статуса ответа": уточнение/ошибка решаются самим Generator через
// error, см. internal/agent/document.go.
type GenerateResult struct {
	AnswerText string
	Sources    []domain.Source
	Findings   []domain.Finding
}

// Generator — вызов LLM с AgentPrompt(document) поверх той же
// LLM-инфраструктуры, что thread.Agent. Порт объявлен здесь;
// internal/agent.Client реализует его новым методом GenerateDocument
// (BACKEND_CODING_STANDARDS.md §1.1 — тот же приём, что thread.Agent,
// объявленный в internal/service/thread и реализуемый тем же internal/agent.Client).
type Generator interface {
	GenerateDocument(ctx context.Context, history []domain.Message) (GenerateResult, error)
}

// Renderer — DocumentsService.Render (Stage 4) поверх уже готового
// grpcclient.Client.RenderDocument — типы Format/Result берутся напрямую из
// grpcclient: не дублировать чужой тип ради формального разделения
// пакетов (BACKEND_CODING_STANDARDS.md §1.1).
type Renderer interface {
	RenderDocument(ctx context.Context, title string, sections []domain.Finding, format grpcclient.RenderFormat) (grpcclient.RenderResult, error)
}

// Storage — presigned-ссылка на скачивание сгенерированного файла (тот же
// принцип, что file.Service: object_key хранится в БД, ссылка
// перевыпускается на каждый ответ, не кешируется).
type Storage interface {
	PresignGetPublic(ctx context.Context, key string, expiry time.Duration) (string, error)
}

// renderFormats — генерация документа всегда рендерит оба формата сразу
// (zan-backend-tz-v2.md §4.4: "FileAttachment(..., output_formats=
// ["pdf","docx"])") — GET /threads/{id}/document?format= затем просто
// отдаёт уже готовый файл, без рендера по требованию.
var renderFormats = []grpcclient.RenderFormat{grpcclient.RenderFormatPDF, grpcclient.RenderFormatDOCX}

// mimeTypeByFormat/allOutputFormats — единая точка соответствия
// grpcclient.RenderFormat <-> MIME-тип/строка формата в API и в БД.
var mimeTypeByFormat = map[grpcclient.RenderFormat]string{
	grpcclient.RenderFormatPDF:  "application/pdf",
	grpcclient.RenderFormatDOCX: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
}

var allOutputFormats = []string{string(grpcclient.RenderFormatPDF), string(grpcclient.RenderFormatDOCX)}

// Service — бизнес-логика генерации документа (BACKEND_PLAN.md Stage 6).
type Service struct {
	threads   ThreadStore
	files     FileStore
	balance   BalanceService
	catalog   ServiceCatalog
	generator Generator
	renderer  Renderer
	storage   Storage
	clock     clock.Clock
	idgen     idgen.Generator
}

// New собирает Service с внедрёнными зависимостями.
func New(threads ThreadStore, files FileStore, balance BalanceService, cat ServiceCatalog, generator Generator, renderer Renderer, storage Storage, clk clock.Clock, ids idgen.Generator) *Service {
	return &Service{
		threads:   threads,
		files:     files,
		balance:   balance,
		catalog:   cat,
		generator: generator,
		renderer:  renderer,
		storage:   storage,
		clock:     clk,
		idgen:     ids,
	}
}

// Generate — POST /threads/{id}/generate-document (zan-backend-tz-v2.md
// §4.4): списывает "doc" (та же логика, что и §4.2 для "qa" — недостаточно
// баланса возвращается как billing.ErrInsufficientBalance, клиент делает
// POST /payments/checkout {items:[{service_id:"doc",qty:1}], thread_id} +
// confirm и повторяет вызов — thread.Service.Resume тут не участвует,
// он активирует только неоплаченный тред (awaiting_payment), для уже
// оплаченного треда это no-op),
// вызывает LLM (AgentPrompt(document)), сохраняет ответ ассистента в тред,
// рендерит PDF и DOCX.
//
// Возврат (Message, filesReady, error) — не (Message, error): неуспех
// рендера (backend-roadmap.md §5.2, последняя строка таблицы: "ошибка
// изолирована к операции генерации файла — текст ответа в чате сохраняется,
// doc-кредит возвращается") — НЕ ошибка транспорта, assistantMsg к этому
// моменту уже закоммичен в БД и должен быть отдан клиенту как обычный
// успешный ответ (тот же принцип, что confirmPaymentHandler делает для
// thread.Service.Resume: платёж уже успешен, тред просто не стартовал
// синхронно вместе с ним — клиент разберётся через отдельный запрос,
// здесь — GET /threads/{id}/document, который вернёт ErrNotGenerated, пока
// клиент не попробует Generate ещё раз). error != nil — операция не
// состоялась вообще, сохранять нечего (не найден/не хватает баланса/сама
// генерация текста не удалась).
func (s *Service) Generate(ctx context.Context, threadID, sessionID string) (domain.Message, bool, error) {
	started := s.clock.Now()
	l := logger.FromContext(ctx).With(slog.String("thread_id", threadID))

	t, err := s.threads.GetByID(ctx, threadID)
	if err != nil {
		return domain.Message{}, false, err
	}
	if t.SessionID != sessionID || t.IsDeleted() {
		return domain.Message{}, false, ErrThreadNotFound
	}
	if _, err := s.catalog.GetServiceByID(ctx, serviceID); err != nil {
		return domain.Message{}, false, fmt.Errorf("document: generate: %w", err)
	}

	if err := s.balance.DebitCredit(ctx, sessionID, serviceID); err != nil {
		return domain.Message{}, false, err
	}
	l.Info("balance_checked", slog.Group("context", slog.String("service_id", serviceID)))

	history, err := s.threads.GetConversation(ctx, threadID)
	if err != nil {
		s.refund(ctx, l, sessionID)
		return domain.Message{}, false, fmt.Errorf("document: load history: %w", err)
	}

	genResult, err := s.generator.GenerateDocument(ctx, history)
	if err != nil {
		s.refund(ctx, l, sessionID)
		return domain.Message{}, false, fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}

	processingTimeMs := int(s.clock.Now().Sub(started).Milliseconds())
	assistantMsg := domain.Message{
		ID:               s.idgen.NewID(),
		ThreadID:         t.ID,
		Sender:           domain.MessageSenderAssistant,
		InputType:        domain.MessageInputTypeText,
		Text:             genResult.AnswerText,
		Sources:          genResult.Sources,
		Findings:         genResult.Findings,
		ProcessingTimeMs: &processingTimeMs,
		CreatedAt:        s.clock.Now(),
	}
	if _, err := s.threads.AppendMessage(ctx, assistantMsg); err != nil {
		// Ничего ещё не показано пользователю — полный возврат кредита, как
		// при неуспехе генерации текста выше.
		s.refund(ctx, l, sessionID)
		return domain.Message{}, false, fmt.Errorf("document: save message: %w", err)
	}
	l.Info("message_saved", slog.Group("context", slog.String("message_id", assistantMsg.ID)))

	if err := s.renderAll(ctx, l, t.Title, genResult.Findings, t.ID, sessionID, assistantMsg.ID); err != nil {
		s.refund(ctx, l, sessionID)
		return assistantMsg, false, nil
	}

	return assistantMsg, true, nil
}

// renderAll — рендерит документ в обоих форматах (renderFormats), сохраняет
// каждый результат отдельной строкой core.file_attachments (см.
// FileStore.CreateGenerated). Останавливается на первой ошибке — уже
// отрендеренные форматы НЕ откатываются (осознанное упрощение: если PDF
// удался, а DOCX упал, PDF остаётся доступным для скачивания — частичный
// успех лучше, чем удаление уже готового файла; кредит всё равно
// возвращается вызывающим кодом, т.к. операция в целом не считается
// успешной, backend-roadmap.md §5.2 не разбирает частичный случай отдельно).
func (s *Service) renderAll(ctx context.Context, l *slog.Logger, title string, sections []domain.Finding, threadID, sessionID, messageID string) error {
	for _, format := range renderFormats {
		if err := s.renderOne(ctx, l, title, sections, format, threadID, sessionID, messageID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) renderOne(ctx context.Context, l *slog.Logger, title string, sections []domain.Finding, format grpcclient.RenderFormat, threadID, sessionID, messageID string) error {
	l.Info("document_render_started", slog.Group("context", slog.String("format", string(format))))
	started := s.clock.Now()

	result, err := s.renderer.RenderDocument(ctx, title, sections, format)
	latencyMs := s.clock.Now().Sub(started).Milliseconds()
	if err != nil {
		l.Error("document_render_completed", slog.Group("context",
			slog.String("format", string(format)),
			slog.Int64("latency_ms", latencyMs),
			slog.String("error", err.Error()),
		))
		return fmt.Errorf("render %s: %w", format, err)
	}

	_, err = s.files.CreateGenerated(ctx, domain.FileAttachment{
		ID:               s.idgen.NewID(),
		SessionID:        sessionID,
		MessageID:        &messageID,
		ThreadID:         &threadID,
		ObjectKey:        result.ObjectKey,
		OriginalName:     documentFileName(title, format),
		MimeType:         mimeTypeByFormat[format],
		Purpose:          domain.FilePurposeGeneratedOutput,
		OutputFormats:    allOutputFormats,
		ProcessingStatus: domain.FileProcessingStatusProcessed,
		CreatedAt:        s.clock.Now(),
	})
	if err != nil {
		l.Error("document_render_completed", slog.Group("context",
			slog.String("format", string(format)),
			slog.Int64("latency_ms", latencyMs),
			slog.String("error", err.Error()),
		))
		return fmt.Errorf("save generated file record (%s): %w", format, err)
	}

	l.Info("document_render_completed", slog.Group("context",
		slog.String("format", string(format)),
		slog.Int64("latency_ms", latencyMs),
	))
	return nil
}

// refund — возврат "doc"-кредита (backend-roadmap.md §5.2). Ошибка возврата
// логируется как CRITICAL — тот же случай, что и
// thread.Service.finishWithError, но здесь она не может вернуться наверх
// как "операция провалилась ещё жёстче": вызывающий код Generate уже
// формирует одну конкретную ошибку (ErrGenerationFailed/ErrRenderFailed),
// расследование зависшего возврата — по логу/trace_id, не по HTTP-ответу.
func (s *Service) refund(ctx context.Context, l *slog.Logger, sessionID string) {
	if err := s.balance.RefundCredit(ctx, sessionID, serviceID, 1); err != nil {
		l.Log(ctx, logger.LevelCritical, "document_credit_refund_failed", slog.Group("context",
			slog.String("service_id", serviceID),
			slog.String("session_id", sessionID),
			slog.String("error", err.Error()),
		))
		return
	}
	l.Warn("credit_refunded", slog.Group("context",
		slog.String("service_id", serviceID),
		slog.String("session_id", sessionID),
	))
}

// GetDocument — GET /threads/{id}/document?format=pdf|docx: последний
// сгенерированный файл треда в этом формате + свежая presigned-ссылка.
func (s *Service) GetDocument(ctx context.Context, threadID, sessionID, format string) (domain.FileAttachment, string, error) {
	renderFormat, err := parseRenderFormat(format)
	if err != nil {
		return domain.FileAttachment{}, "", ErrInvalidFormat
	}

	t, err := s.threads.GetByID(ctx, threadID)
	if err != nil {
		return domain.FileAttachment{}, "", err
	}
	if t.SessionID != sessionID || t.IsDeleted() {
		return domain.FileAttachment{}, "", ErrThreadNotFound
	}

	f, err := s.files.GetLatestGenerated(ctx, threadID, mimeTypeByFormat[renderFormat])
	if err != nil {
		return domain.FileAttachment{}, "", fmt.Errorf("%w: %v", ErrNotGenerated, err)
	}

	url, err := s.storage.PresignGetPublic(ctx, f.ObjectKey, publicPresignTTL)
	if err != nil {
		return domain.FileAttachment{}, "", fmt.Errorf("document: presign: %w", err)
	}
	return f, url, nil
}

func parseRenderFormat(s string) (grpcclient.RenderFormat, error) {
	switch grpcclient.RenderFormat(s) {
	case grpcclient.RenderFormatPDF:
		return grpcclient.RenderFormatPDF, nil
	case grpcclient.RenderFormatDOCX:
		return grpcclient.RenderFormatDOCX, nil
	default:
		return "", ErrInvalidFormat
	}
}

// documentFileName — имя файла для скачивания, по названию треда (то же
// значение, что уже показано пользователю как заголовок обращения).
func documentFileName(title string, format grpcclient.RenderFormat) string {
	if title == "" {
		title = "Документ"
	}
	return title + "." + string(format)
}
