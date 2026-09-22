// Package thread — юзкейсы переписки (BACKEND_PLAN.md Stage 3): создание
// Thread/Message, статус-машина (domain.ThreadStatus), списание/возврат
// баланса за услугу треда, синхронный вызов Agent (internal/agent.Client,
// Stage 5 — реальный вызов LLM, до этого — StubAgent, Stage 3, удалён
// вместе с подключением реального агента). Оркестрирует четыре порта —
// Repository, BalanceService, ServiceCatalog, Agent — все объявлены здесь,
// где используются, реализации живут в internal/repo,
// internal/service/billing, internal/service/catalog, internal/agent
// (BACKEND_CODING_STANDARDS.md §1.1).
package thread

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/billing"
)

const (
	// ListPageSize — размер страницы GET /threads. Не задано продуктом —
	// рекомендация архитектора для MVP (тот же приём, что и у
	// cmd/api/main.go#sessionCreateRatePerSecond), пересмотреть по метрикам прод.
	ListPageSize = 20

	// TitleMaxRunes — обрезка title до ~60 символов из первого сообщения
	// (zan-backend-tz-v2.md §4.5).
	TitleMaxRunes = 60

	// previewMaxRunes — короткая выжимка ответа ассистента в preview_text,
	// не весь answer_text (§4.5: "короткое превью").
	previewMaxRunes = 140

	// Системные тексты preview_text для нетерминальных статусов —
	// zan-backend-tz-v2.md §4.5, буквально: "Ассистент готовит ответ…" /
	// "Запрос в очереди…". Локализация (ru/kz) вне скоупа backend на этом
	// этапе — тот же прецедент, что и apierror-сообщения (например,
	// billing.go "Оплата не прошла. Попробовать снова"), всегда на русском.
	previewQueued     = "Запрос в очереди…"
	previewProcessing = "Ассистент готовит ответ…"
	previewError      = "Не удалось обработать запрос"
	previewCanceled   = "Обращение отменено"
)

var (
	// ErrThreadNotFound — треда с таким id нет, либо он принадлежит другой
	// сессии, либо soft-deleted — во всех трёх случаях один и тот же ответ
	// (тот же принцип, что у billing.ErrPaymentNotFound: не подтверждать
	// существование чужого id).
	ErrThreadNotFound = errors.New("thread: not found")

	// ErrMessageNotFound — сообщения с таким id нет, либо оно принадлежит
	// треду другой сессии (POST /messages/{id}/feedback).
	ErrMessageNotFound = errors.New("thread: message not found")

	// ErrThreadClosed — free_until истёк (zan-backend-tz-v2.md §4.3): новое
	// сообщение по теме — новый тред.
	ErrThreadClosed = errors.New("thread: closed, start a new thread")

	// ErrThreadNotActive — тред в терминальном статусе (error/canceled),
	// продолжить его нельзя — по аналогии с §4.3, начинается новый тред.
	ErrThreadNotActive = errors.New("thread: not active, start a new thread")

	// ErrPaymentRequired — POST /threads/{id}/messages на тред, который
	// ещё не оплачен (queued, is_paid=false) — клиент должен сперва
	// провести checkout/confirm за этот thread_id (zan-backend-tz-v2.md §4.2 п.2).
	ErrPaymentRequired = errors.New("thread: payment required")

	// ErrThreadBusy — тред уже обрабатывается (status=processing).
	// Agent.Process (Stage 5 — internal/agent.Client, реальный вызов LLM)
	// по-прежнему синхронный, в пределах одного HTTP-запроса (Strategy,
	// смена реализации не потребовала менять этот контракт) — практически
	// недостижимо тем же способом, что и раньше со StubAgent, разве что
	// два параллельных запроса на один thread_id физически совпадут по
	// времени. Переход на асинхронную обработку (если понадобится из-за
	// латентности LLM) — пересмотр вне скоупа Stage 5, статус-машина уже
	// на него рассчитана.
	ErrThreadBusy = errors.New("thread: already processing")

	// ErrCannotCancel — POST /threads/{id}/cancel вне допустимых статусов
	// (zan-backend-tz-v2.md §3.2: "до оплаты или во время обработки" —
	// только Queued/Processing, см. domain.ThreadStatus.CanTransitionTo).
	ErrCannotCancel = errors.New("thread: cannot be canceled in current status")

	// ErrUnsupportedInputType — voice/file приняты доменной моделью (схема
	// БД это разрешает с Stage 1), но реально обрабатываются только с
	// Stage 4 (STT/извлечение файлов) — до тех пор явная ошибка, не тихая
	// заглушка (BACKEND_CODING_STANDARDS.md, принцип "чистая замена").
	ErrUnsupportedInputType = errors.New("thread: unsupported input type at this stage")

	// ErrEmptyText — text пуст после trim.
	ErrEmptyText = errors.New("thread: text must not be empty")

	// ErrInvalidStatusFilter — GET /threads?status= со значением вне
	// domain.ThreadStatus.
	ErrInvalidStatusFilter = errors.New("thread: invalid status filter")
)

// ListFilter — параметры GET /threads?status=&search=&page=
// (zan-backend-tz-v2.md §3.2).
type ListFilter struct {
	Status   *domain.ThreadStatus
	Search   string
	Page     int
	PageSize int
}

// Repository — порт доступа к Thread/Message. Составные операции
// (CreateThread, AppendMessage, UpdateStatus, ActivatePaid) — по тому же
// принципу, что и billing.Repository (BACKEND_CODING_STANDARDS.md §1.1):
// осмысленные атомарные юзкейсы, не набор сырых CRUD-примитивов, которые
// service собирал бы в транзакцию сам.
type Repository interface {
	// CreateThread — атомарно вставляет Thread + первое Message.
	CreateThread(ctx context.Context, t domain.Thread, firstMessage domain.Message) (domain.Thread, domain.Message, error)

	GetByID(ctx context.Context, id string) (domain.Thread, error)
	// GetMessages — все сообщения треда, от старых к новым.
	GetMessages(ctx context.Context, threadID string) ([]domain.Message, error)
	ListThreads(ctx context.Context, sessionID string, filter ListFilter) ([]domain.Thread, int, error)

	// AppendMessage — вставляет сообщение и атомарно увеличивает
	// thread.message_count/last_message_at, возвращает обновлённый Thread.
	AppendMessage(ctx context.Context, msg domain.Message) (domain.Thread, error)

	// UpdateStatus — переход status: один из from -> to, атомарно
	// (WHERE status = ANY(from)), вместе с preview_text. Тот же паттерн,
	// что billing_repo.ConfirmPayment: ok=false — статус уже не из from
	// (гонка/повторный вызов), не ошибка транспорта.
	UpdateStatus(ctx context.Context, id string, from []domain.ThreadStatus, to domain.ThreadStatus, previewText string) (domain.Thread, bool, error)

	// ActivatePaid — WHERE status=Queued AND is_paid=false: списание уже
	// произошло (billing.ConfirmPayment), проставляет is_paid/paid_at/
	// free_until и переводит в Processing одной атомарной операцией
	// (zan-backend-tz-v2.md §4.2 п.2, "купил и тут же потратил"). ok=false
	// — тред уже активирован (повторный confirm) или не в Queued.
	ActivatePaid(ctx context.Context, id string, paidAt, freeUntil time.Time, previewText string) (domain.Thread, bool, error)

	// CloseIfExpired — идемпотентно проставляет closed_at, если ещё не
	// проставлен (WHERE closed_at IS NULL). true — реально закрыл сейчас.
	CloseIfExpired(ctx context.Context, id string, now time.Time) (bool, error)

	// SoftDelete — WHERE deleted_at IS NULL, идемпотентно. false — уже был
	// удалён (репозиторий не различает "не найден"/"уже удалён" наружу —
	// вызывающий Service решает, что это ErrThreadNotFound).
	SoftDelete(ctx context.Context, id string, now time.Time) (bool, error)

	// SetMessageFeedback — обновляет feedback только если сообщение
	// принадлежит треду этой sessionID (ownership проверяется одним
	// запросом, не отдельным SELECT — тот же приём, что у GetPaymentByID
	// не применяется, но по духу billing.GetPayment: не подтверждать
	// существование чужого id).
	SetMessageFeedback(ctx context.Context, messageID, sessionID string, feedback domain.MessageFeedback) (domain.Message, error)
}

// BalanceService — то немногое, что thread нужно от баланса, чтобы
// списать/вернуть услугу (zan-backend-tz-v2.md §4.2). Порт объявлен здесь,
// а не в internal/service/billing, где реализуется — *billing.Service
// удовлетворяет ему структурно, без импорта пакета thread
// (BACKEND_CODING_STANDARDS.md §1.1, тот же приём, что billing.CatalogReader).
type BalanceService interface {
	// DebitCredit — см. billing.Service.DebitCredit. Возвращает
	// billing.ErrInsufficientBalance, когда баланс исчерпан — это не ошибка
	// транспорта, а обычный сценарий "тред создаётся неоплаченным"
	// (zan-backend-tz-v2.md §4.2 п.2), поэтому thread.Service явно проверяет
	// именно этот сентинел (errors.Is), а не любую ошибку интерфейса.
	DebitCredit(ctx context.Context, sessionID, serviceID string) error
	RefundCredit(ctx context.Context, sessionID, serviceID string, qty int) error
	GetBalance(ctx context.Context, sessionID string) ([]domain.UserCredit, error)
}

// ServiceCatalog — то немногое, что thread нужно от каталога: убедиться,
// что service_id существует, перед тем как пытаться списать баланс.
type ServiceCatalog interface {
	GetServiceByID(ctx context.Context, id string) (domain.Service, error)
}

// FileAttacher — то немногое, что thread нужно от файлов (Stage 4): владение
// уже загруженных file_ids (POST /files/upload прошёл раньше) и привязка к
// только что созданному сообщению. Порт объявлен здесь, а не в
// internal/service/file, где реализуется — *file.Service удовлетворяет ему
// структурно (BACKEND_CODING_STANDARDS.md §1.1, тот же приём, что
// BalanceService/ServiceCatalog).
type FileAttacher interface {
	// ValidateAvailable — см. file.Service.ValidateAvailable. Возвращает
	// file.ErrNotFound, если хотя бы один id не существует/чужой/уже
	// привязан — thread.Service не различает эти случаи, только пробрасывает
	// ошибку выше (httpserver мапит её на 404, не подтверждая наружу, какая
	// именно причина).
	ValidateAvailable(ctx context.Context, sessionID string, fileIDs []string) error
	// AttachToMessage — вызывается ПОСЛЕ успешного создания сообщения, не в
	// одной транзакции с ним — тот же принятый компромисс, что
	// billing.ConfirmPayment -> ActivateAfterPayment (Stage 3): изолированный
	// неуспех здесь логируется, но не откатывает уже созданное сообщение.
	AttachToMessage(ctx context.Context, messageID string, fileIDs []string) error
}

// Service — бизнес-логика переписки (BACKEND_PLAN.md Stage 3/4).
type Service struct {
	repo         Repository
	balance      BalanceService
	catalog      ServiceCatalog
	files        FileAttacher
	agent        Agent
	clock        clock.Clock
	idgen        idgen.Generator
	freeUntilTTL time.Duration
}

// New собирает Service с внедрёнными зависимостями. freeUntilTTL —
// конфигурируемое окно бесплатных уточнений (zan-backend-tz-v2.md §4.3,
// config.Config.ThreadFreeUntil, дефолт 72 часа).
func New(repo Repository, balance BalanceService, cat ServiceCatalog, files FileAttacher, agent Agent, clk clock.Clock, ids idgen.Generator, freeUntilTTL time.Duration) *Service {
	return &Service{
		repo:         repo,
		balance:      balance,
		catalog:      cat,
		files:        files,
		agent:        agent,
		clock:        clk,
		idgen:        ids,
		freeUntilTTL: freeUntilTTL,
	}
}

// CreateThreadRequest — вход POST /threads (zan-backend-tz-v2.md §3.2:
// "{service_id, text, input_type, file_ids?}").
type CreateThreadRequest struct {
	SessionID string
	Language  domain.Language
	ServiceID string
	Text      string
	InputType domain.MessageInputType
	FileIDs   []string
}

// CreateThread — POST /threads. Если баланс sessionID по ServiceID
// достаточен — списывает 1 единицу и синхронно запускает обработку
// (Queued -> Processing -> Done/Clarify/Error, zan-backend-tz-v2.md §4.2 п.2);
// иначе создаёт тред неоплаченным (Queued, is_paid=false) — клиент
// проводит POST /payments/checkout {..., thread_id} и Stage3-хук на
// confirm (ActivateAfterPayment) достроит остальное.
func (s *Service) CreateThread(ctx context.Context, req CreateThreadRequest) (domain.Thread, error) {
	started := s.clock.Now()
	l := logger.FromContext(ctx)
	l.Info("request_received", slog.Group("context",
		slog.String("session_id", req.SessionID),
		slog.String("thread_id", "new"),
		slog.String("input_type", string(req.InputType)),
	))

	text, err := s.validateMessageInput(ctx, req.SessionID, req.InputType, req.Text, req.FileIDs)
	if err != nil {
		return domain.Thread{}, err
	}
	if _, err := s.catalog.GetServiceByID(ctx, req.ServiceID); err != nil {
		return domain.Thread{}, err
	}

	paid, err := s.tryDebit(ctx, l, req.SessionID, req.ServiceID)
	if err != nil {
		return domain.Thread{}, fmt.Errorf("thread: create: %w", err)
	}

	now := s.clock.Now()
	newThread := domain.Thread{
		ID:            s.idgen.NewID(),
		SessionID:     req.SessionID,
		ServiceID:     req.ServiceID,
		Status:        domain.ThreadStatusQueued,
		Title:         deriveTitle(titleSource(text)),
		PreviewText:   previewQueued,
		MessageCount:  1,
		CreatedAt:     now,
		LastMessageAt: &now,
	}
	if paid {
		newThread.IsPaid = true
		newThread.PaidAt = &now
		freeUntil := now.Add(s.freeUntilTTL)
		newThread.FreeUntil = &freeUntil
	}
	firstMessage := domain.Message{
		ID:        s.idgen.NewID(),
		Sender:    domain.MessageSenderUser,
		InputType: req.InputType,
		Text:      text,
		CreatedAt: now,
	}

	created, savedMsg, err := s.repo.CreateThread(ctx, newThread, firstMessage)
	if err != nil {
		return domain.Thread{}, fmt.Errorf("thread: create: %w", err)
	}
	l = l.With(slog.String("thread_id", created.ID))
	l.Info("message_saved", slog.Group("context", slog.String("message_id", savedMsg.ID)))
	s.attachFiles(ctx, l, savedMsg.ID, req.FileIDs)

	if !paid {
		l.Info("payment_required", slog.Group("context", slog.String("service_id", req.ServiceID)))
		s.logResponseSent(l, started, created.Status)
		return created, nil
	}

	begun, ok, err := s.beginProcessing(ctx, l, created.ID, domain.ThreadStatusQueued)
	if err != nil {
		return domain.Thread{}, err
	}
	if !ok {
		return s.refetch(ctx, created.ID)
	}
	return s.runAgentAndFinish(ctx, l, begun, true, req.Language, started)
}

// GetThread — GET /threads/{id}: тред + все сообщения, только владелец
// сессии, не soft-deleted.
func (s *Service) GetThread(ctx context.Context, id, sessionID string) (domain.Thread, []domain.Message, error) {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Thread{}, nil, err
	}
	if t.SessionID != sessionID || t.IsDeleted() {
		return domain.Thread{}, nil, ErrThreadNotFound
	}
	msgs, err := s.repo.GetMessages(ctx, id)
	if err != nil {
		return domain.Thread{}, nil, fmt.Errorf("thread: get messages: %w", err)
	}
	return t, msgs, nil
}

// ListThreads — GET /threads?status=&search=&page=, только треды
// sessionID, без soft-deleted (репозиторий это гарантирует).
func (s *Service) ListThreads(ctx context.Context, sessionID, statusFilter, search string, page int) ([]domain.Thread, int, error) {
	var status *domain.ThreadStatus
	if statusFilter != "" {
		parsed, err := domain.ParseThreadStatus(statusFilter)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: %q", ErrInvalidStatusFilter, statusFilter)
		}
		status = &parsed
	}
	if page < 1 {
		page = 1
	}
	items, total, err := s.repo.ListThreads(ctx, sessionID, ListFilter{
		Status:   status,
		Search:   strings.TrimSpace(search),
		Page:     page,
		PageSize: ListPageSize,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("thread: list: %w", err)
	}
	return items, total, nil
}

// AddMessageRequest — вход POST /threads/{id}/messages
// (zan-backend-tz-v2.md §3.2: "{text, input_type, file_ids?}").
type AddMessageRequest struct {
	ThreadID  string
	SessionID string
	Language  domain.Language
	Text      string
	InputType domain.MessageInputType
	FileIDs   []string
}

// AddMessage — POST /threads/{id}/messages: уточнение внутри треда.
// Бесплатно, пока status IN (Done, Clarify) и now() < free_until
// (zan-backend-tz-v2.md §4.3) — баланс не списывается повторно.
func (s *Service) AddMessage(ctx context.Context, req AddMessageRequest) (domain.Thread, error) {
	started := s.clock.Now()
	l := logger.FromContext(ctx).With(slog.String("thread_id", req.ThreadID))
	l.Info("request_received", slog.Group("context",
		slog.String("session_id", req.SessionID),
		slog.String("input_type", string(req.InputType)),
	))

	text, err := s.validateMessageInput(ctx, req.SessionID, req.InputType, req.Text, req.FileIDs)
	if err != nil {
		return domain.Thread{}, err
	}

	t, err := s.repo.GetByID(ctx, req.ThreadID)
	if err != nil {
		return domain.Thread{}, err
	}
	if t.SessionID != req.SessionID || t.IsDeleted() {
		return domain.Thread{}, ErrThreadNotFound
	}

	now := s.clock.Now()
	if t.IsClosed(now) {
		if t.ClosedAt == nil {
			if _, err := s.repo.CloseIfExpired(ctx, t.ID, now); err != nil {
				return domain.Thread{}, fmt.Errorf("thread: close expired: %w", err)
			}
		}
		return domain.Thread{}, ErrThreadClosed
	}

	switch t.Status {
	case domain.ThreadStatusDone, domain.ThreadStatusClarify:
		// продолжение диалога — бесплатно в пределах free_until, уже проверено выше.
	case domain.ThreadStatusQueued:
		return domain.Thread{}, ErrPaymentRequired
	case domain.ThreadStatusProcessing:
		return domain.Thread{}, ErrThreadBusy
	default: // Error, Canceled — терминальные статусы.
		return domain.Thread{}, ErrThreadNotActive
	}
	fromStatus := t.Status

	msg := domain.Message{
		ID:        s.idgen.NewID(),
		ThreadID:  t.ID,
		Sender:    domain.MessageSenderUser,
		InputType: req.InputType,
		Text:      text,
		CreatedAt: now,
	}
	updated, err := s.repo.AppendMessage(ctx, msg)
	if err != nil {
		return domain.Thread{}, fmt.Errorf("thread: append message: %w", err)
	}
	l.Info("message_saved", slog.Group("context", slog.String("message_id", msg.ID)))
	s.attachFiles(ctx, l, msg.ID, req.FileIDs)

	begun, ok, err := s.beginProcessing(ctx, l, updated.ID, fromStatus)
	if err != nil {
		return domain.Thread{}, err
	}
	if !ok {
		return s.refetch(ctx, updated.ID)
	}
	refundEligible := fromStatus != domain.ThreadStatusDone
	return s.runAgentAndFinish(ctx, l, begun, refundEligible, req.Language, started)
}

// ActivateAfterPayment — хук, вызываемый httpserver сразу после успешного
// billing.Service.ConfirmPayment для платежа с непустым ThreadID
// (zan-backend-tz-v2.md §4.2 п.2: "после confirm начисляется 1 единица и
// сразу списывается по логике выше"). Идемпотентна: повторный вызов
// (повторный confirm) на уже активированный тред — no-op, отдаёт текущее
// состояние, не ошибку.
func (s *Service) ActivateAfterPayment(ctx context.Context, threadID, sessionID string, lang domain.Language) (domain.Thread, error) {
	started := s.clock.Now()
	t, err := s.repo.GetByID(ctx, threadID)
	if err != nil {
		return domain.Thread{}, err
	}
	if t.SessionID != sessionID || t.IsDeleted() {
		return domain.Thread{}, ErrThreadNotFound
	}
	if t.Status != domain.ThreadStatusQueued || t.IsPaid {
		return t, nil
	}

	l := logger.FromContext(ctx).With(slog.String("thread_id", t.ID))
	now := s.clock.Now()
	freeUntil := now.Add(s.freeUntilTTL)
	activated, ok, err := s.repo.ActivatePaid(ctx, t.ID, now, freeUntil, previewProcessing)
	if err != nil {
		return domain.Thread{}, fmt.Errorf("thread: activate after payment: %w", err)
	}
	if !ok {
		return s.refetch(ctx, t.ID)
	}
	l.Info("payment_confirmed", slog.Group("context", slog.String("service_id", t.ServiceID)))
	logTransition(l, domain.ThreadStatusQueued, domain.ThreadStatusProcessing)

	return s.runAgentAndFinish(ctx, l, activated, true, lang, started)
}

// CancelThread — POST /threads/{id}/cancel: статус -> Canceled, только из
// Queued/Processing (zan-backend-tz-v2.md §3.2 — "до оплаты или во время
// обработки", domain.ThreadStatus.CanTransitionTo). Кредит не возвращается
// (в отличие от Error) — не задано ТЗ явно для отмены, только для ошибки
// (§4.2 п.3).
func (s *Service) CancelThread(ctx context.Context, id, sessionID string) (domain.Thread, error) {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Thread{}, err
	}
	if t.SessionID != sessionID || t.IsDeleted() {
		return domain.Thread{}, ErrThreadNotFound
	}
	if !t.Status.CanTransitionTo(domain.ThreadStatusCanceled) {
		return domain.Thread{}, fmt.Errorf("%w: from %q", ErrCannotCancel, t.Status)
	}

	updated, ok, err := s.repo.UpdateStatus(ctx, id, []domain.ThreadStatus{t.Status}, domain.ThreadStatusCanceled, previewCanceled)
	if err != nil {
		return domain.Thread{}, fmt.Errorf("thread: cancel: %w", err)
	}
	if !ok {
		return domain.Thread{}, fmt.Errorf("%w: status changed concurrently", ErrCannotCancel)
	}
	logTransition(logger.FromContext(ctx).With(slog.String("thread_id", id)), t.Status, domain.ThreadStatusCanceled)
	return updated, nil
}

// DeleteThread — DELETE /threads/{id}: soft delete (zan-backend-tz-v2.md §4.5).
func (s *Service) DeleteThread(ctx context.Context, id, sessionID string) error {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if t.SessionID != sessionID || t.IsDeleted() {
		return ErrThreadNotFound
	}
	ok, err := s.repo.SoftDelete(ctx, id, s.clock.Now())
	if err != nil {
		return fmt.Errorf("thread: delete: %w", err)
	}
	if !ok {
		return ErrThreadNotFound
	}
	return nil
}

// SetMessageFeedback — POST /messages/{id}/feedback
// (zan-backend-tz-v2.md §3.6).
func (s *Service) SetMessageFeedback(ctx context.Context, messageID, sessionID string, feedback domain.MessageFeedback) (domain.Message, error) {
	msg, err := s.repo.SetMessageFeedback(ctx, messageID, sessionID, feedback)
	if err != nil {
		return domain.Message{}, err
	}
	return msg, nil
}

// tryDebit — попытка списать 1 единицу serviceID с баланса sessionID,
// логирует balance_checked (zan-backend-tz-v3.md §6.2 п.2) с
// credit_before/credit_after. paid=false — баланс исчерпан, обычный
// сценарий (billing.ErrInsufficientBalance), не ошибка.
func (s *Service) tryDebit(ctx context.Context, l *slog.Logger, sessionID, serviceID string) (paid bool, err error) {
	before, err := s.balance.GetBalance(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("get balance: %w", err)
	}
	qtyBefore := creditQuantity(before, serviceID)

	err = s.balance.DebitCredit(ctx, sessionID, serviceID)
	switch {
	case err == nil:
		logBalanceChecked(l, serviceID, qtyBefore, qtyBefore-1)
		return true, nil
	case errors.Is(err, billing.ErrInsufficientBalance):
		logBalanceChecked(l, serviceID, qtyBefore, qtyBefore)
		return false, nil
	default:
		return false, fmt.Errorf("debit credit: %w", err)
	}
}

func logBalanceChecked(l *slog.Logger, serviceID string, before, after int) {
	l.Info("balance_checked", slog.Group("context",
		slog.String("service_id", serviceID),
		slog.Int("credit_before", before),
		slog.Int("credit_after", after),
	))
}

func creditQuantity(credits []domain.UserCredit, serviceID string) int {
	for _, c := range credits {
		if c.ServiceID == serviceID {
			return c.Quantity
		}
	}
	return 0
}

// beginProcessing — гарантированный переход from -> Processing
// (WHERE status=from), логирует thread_status_changed при успехе.
// ok=false — статус уже не from (гонка/повторный вызов), не ошибка.
func (s *Service) beginProcessing(ctx context.Context, l *slog.Logger, id string, from domain.ThreadStatus) (domain.Thread, bool, error) {
	t, ok, err := s.repo.UpdateStatus(ctx, id, []domain.ThreadStatus{from}, domain.ThreadStatusProcessing, previewProcessing)
	if err != nil {
		return domain.Thread{}, false, fmt.Errorf("thread: begin processing: %w", err)
	}
	if ok {
		logTransition(l, from, domain.ThreadStatusProcessing)
	}
	return t, ok, nil
}

// runAgentAndFinish — тред уже в Processing (переход в него сделан вызывающим
// кодом — beginProcessing или ActivatePaid, у обоих своя механика
// гарантированного перехода). Загружает историю, синхронно вызывает
// Agent.Process, переводит в Done/Clarify/Error; при Error — возвращает
// кредит, если refundEligible (тред ещё ни разу не получил успешный ответ —
// zan-backend-tz-v2.md §4.2 п.3: повторная бесплатная попытка внутри
// free_until не пере-возвращает уже потраченный при первом успехе кредит).
//
// Этапы 7/8/9 из backend-roadmap.md §6.2 (rag_search_*/llm_call_*) здесь
// НЕ логируются — этот уровень не знает промпта/RAG-кандидатов, которые
// реально использовались (только internal/agent.Client их знает, Stage 5),
// поэтому логирует их сам internal/agent.Client.Process. Здесь остаются
// только этапы, которые действительно видны на этом уровне: переход
// статуса, сохранение сообщения, возврат кредита, response_sent.
func (s *Service) runAgentAndFinish(ctx context.Context, l *slog.Logger, t domain.Thread, refundEligible bool, lang domain.Language, started time.Time) (domain.Thread, error) {
	history, err := s.repo.GetMessages(ctx, t.ID)
	if err != nil {
		return domain.Thread{}, fmt.Errorf("thread: load history: %w", err)
	}

	result, err := s.agent.Process(ctx, AgentRequest{ThreadID: t.ID, ServiceID: t.ServiceID, Language: lang, History: history})
	if err != nil {
		return s.finishWithError(ctx, l, t, refundEligible, started)
	}

	switch result.Status {
	case AgentResultDone, AgentResultClarify:
		return s.finishWithAnswer(ctx, l, t, result, started)
	case AgentResultError:
		return s.finishWithError(ctx, l, t, refundEligible, started)
	default:
		return domain.Thread{}, fmt.Errorf("thread: unknown agent result status %q", result.Status)
	}
}

func (s *Service) finishWithAnswer(ctx context.Context, l *slog.Logger, t domain.Thread, result AgentResult, started time.Time) (domain.Thread, error) {
	to := domain.ThreadStatusDone
	if result.Status == AgentResultClarify {
		to = domain.ThreadStatusClarify
	}

	processingTimeMs := int(s.clock.Now().Sub(started).Milliseconds())
	assistantMsg := domain.Message{
		ID:                s.idgen.NewID(),
		ThreadID:          t.ID,
		Sender:            domain.MessageSenderAssistant,
		InputType:         domain.MessageInputTypeText,
		Text:              result.AnswerText,
		Sources:           result.Sources,
		Findings:          result.Findings,
		UnverifiedSources: result.UnverifiedSources,
		ProcessingTimeMs:  &processingTimeMs,
		CreatedAt:         s.clock.Now(),
	}
	if _, err := s.repo.AppendMessage(ctx, assistantMsg); err != nil {
		return domain.Thread{}, fmt.Errorf("thread: save assistant message: %w", err)
	}
	l.Info("message_saved", slog.Group("context", slog.String("message_id", assistantMsg.ID)))

	final, ok, err := s.repo.UpdateStatus(ctx, t.ID, []domain.ThreadStatus{domain.ThreadStatusProcessing}, to, previewFromAnswer(result.AnswerText))
	if err != nil {
		return domain.Thread{}, fmt.Errorf("thread: finish processing: %w", err)
	}
	if !ok {
		if final, err = s.refetch(ctx, t.ID); err != nil {
			return domain.Thread{}, err
		}
	} else {
		logTransition(l, domain.ThreadStatusProcessing, to)
	}
	s.logResponseSent(l, started, final.Status)
	return final, nil
}

func (s *Service) finishWithError(ctx context.Context, l *slog.Logger, t domain.Thread, refundEligible bool, started time.Time) (domain.Thread, error) {
	final, ok, err := s.repo.UpdateStatus(ctx, t.ID, []domain.ThreadStatus{domain.ThreadStatusProcessing}, domain.ThreadStatusError, previewError)
	if err != nil {
		return domain.Thread{}, fmt.Errorf("thread: mark error: %w", err)
	}
	if !ok {
		if final, err = s.refetch(ctx, t.ID); err != nil {
			return domain.Thread{}, err
		}
	} else {
		logTransition(l, domain.ThreadStatusProcessing, domain.ThreadStatusError)
	}

	if refundEligible {
		if err := s.balance.RefundCredit(ctx, final.SessionID, final.ServiceID, 1); err != nil {
			// Тред уже помечен Error, но возврат кредита не удался — деньги
			// "зависли". Отдаём ошибку наверх (httpserver -> 500):
			// расследуется по trace_id/thread_id в логах, повторный запрос
			// клиента (GET /threads/{id}) увидит status=error и может
			// послужить сигналом для ручного разбора.
			return domain.Thread{}, fmt.Errorf("thread: refund credit: %w", err)
		}
		l.Warn("credit_refunded", slog.Group("context",
			slog.String("service_id", final.ServiceID),
			slog.String("session_id", final.SessionID),
		))
	}
	s.logResponseSent(l, started, domain.ThreadStatusError)
	return final, nil
}

func (s *Service) logResponseSent(l *slog.Logger, started time.Time, status domain.ThreadStatus) {
	l.Info("response_sent", slog.Group("context",
		slog.String("status", string(status)),
		slog.Int64("duration_ms", s.clock.Now().Sub(started).Milliseconds()),
	))
}

func (s *Service) refetch(ctx context.Context, id string) (domain.Thread, error) {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Thread{}, fmt.Errorf("thread: refetch after race: %w", err)
	}
	return t, nil
}

func logTransition(l *slog.Logger, from, to domain.ThreadStatus) {
	l.Info("thread_status_changed", slog.Group("context",
		slog.String("from_status", string(from)),
		slog.String("to_status", string(to)),
	))
}

func previewFromAnswer(answer string) string {
	return truncateRunes(strings.TrimSpace(answer), previewMaxRunes)
}

func deriveTitle(text string) string {
	return truncateRunes(text, TitleMaxRunes)
}

// titleSource — фолбэк для деривации title, когда сообщение состоит только
// из вложения (input_type=file без текста, Stage 4) — deriveTitle("") дал
// бы пустой заголовок треда, что хуже, чем нейтральная подпись.
func titleSource(text string) string {
	if text == "" {
		return "Вложение без текста"
	}
	return text
}

// validateMessageInput — общая валидация {text, input_type, file_ids?} для
// CreateThread/AddMessage (Stage 4, zan-backend-tz-v2.md §3.2): input_type
// должен быть одним из известных значений, и сообщение обязано нести хоть
// что-то — непустой текст ИЛИ хотя бы один файл (voice уже пришёл сюда
// расшифрованным текстом через POST /voice/transcribe, zan-backend-tz-v2.md
// §4.7 — для thread.Service он неотличим от text). Возвращает
// провалидированный (trimmed) текст.
func (s *Service) validateMessageInput(ctx context.Context, sessionID string, inputType domain.MessageInputType, rawText string, fileIDs []string) (string, error) {
	switch inputType {
	case domain.MessageInputTypeText, domain.MessageInputTypeVoice, domain.MessageInputTypeFile:
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedInputType, inputType)
	}

	text := strings.TrimSpace(rawText)
	if text == "" && len(fileIDs) == 0 {
		return "", ErrEmptyText
	}
	if len(fileIDs) > 0 {
		if err := s.files.ValidateAvailable(ctx, sessionID, fileIDs); err != nil {
			return "", err
		}
	}
	return text, nil
}

// attachFiles — привязывает fileIDs к уже сохранённому сообщению. Отдельный
// шаг после успешного создания сообщения (не в общей транзакции — см.
// FileAttacher) — неуспех логируется и не откатывает уже созданное
// сообщение, тот же принятый компромисс, что ActivateAfterPayment (Stage 3).
func (s *Service) attachFiles(ctx context.Context, l *slog.Logger, messageID string, fileIDs []string) {
	if len(fileIDs) == 0 {
		return
	}
	if err := s.files.AttachToMessage(ctx, messageID, fileIDs); err != nil {
		l.Error("file_attach_failed", slog.Group("context",
			slog.String("message_id", messageID),
			slog.String("error", err.Error()),
		))
	}
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
