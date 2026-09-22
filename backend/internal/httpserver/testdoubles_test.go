package httpserver_test

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"zan-backend/internal/domain"
	"zan-backend/internal/grpcclient"
	"zan-backend/internal/service/analytics"
	"zan-backend/internal/service/billing"
	"zan-backend/internal/service/catalog"
	"zan-backend/internal/service/document"
	"zan-backend/internal/service/prompt"
	"zan-backend/internal/service/thread"
)

// fakeCatalogRepo/fakeBillingRepo — in-memory реализации catalog.Repository/
// billing.Repository для юнит-тестов httpserver, без реального Postgres
// (тот тестируется отдельно, internal/repo — testcontainers). Предзаполнены
// qa/doc — тем же сид-набором, что и migrations/000003_stage2_billing.up.sql.

type fakeCatalogRepo struct {
	mu       sync.Mutex
	services map[string]domain.Service
	tariffs  map[string]domain.Tariff
	nextSort int
}

func newFakeCatalogRepo() *fakeCatalogRepo {
	return &fakeCatalogRepo{
		services: map[string]domain.Service{
			"qa":  {ID: "qa", TypeLabel: "Консультация", Name: "Вопрос-ответ", PriceTenge: 2900, IsActive: true},
			"doc": {ID: "doc", TypeLabel: "Документ", Name: "Подготовка документа", PriceTenge: 4900, IsActive: true},
		},
		tariffs: make(map[string]domain.Tariff),
	}
}

func (r *fakeCatalogRepo) ListServices(context.Context) ([]domain.Service, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Service, 0, len(r.services))
	for _, s := range r.services {
		out = append(out, s)
	}
	return out, nil
}

func (r *fakeCatalogRepo) GetServiceByID(_ context.Context, id string) (domain.Service, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.services[id]
	if !ok {
		return domain.Service{}, catalog.ErrServiceNotFound
	}
	return s, nil
}

func (r *fakeCatalogRepo) UpdateService(_ context.Context, id string, priceTenge int, isActive bool) (domain.Service, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.services[id]
	if !ok {
		return domain.Service{}, catalog.ErrServiceNotFound
	}
	s.PriceTenge = priceTenge
	s.IsActive = isActive
	r.services[id] = s
	return s, nil
}

func (r *fakeCatalogRepo) ListTariffs(context.Context) ([]domain.Tariff, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Tariff, 0, len(r.tariffs))
	for _, t := range r.tariffs {
		out = append(out, t)
	}
	return out, nil
}

func (r *fakeCatalogRepo) GetTariffByID(_ context.Context, id string) (domain.Tariff, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tariffs[id]
	if !ok {
		return domain.Tariff{}, catalog.ErrTariffNotFound
	}
	return t, nil
}

func (r *fakeCatalogRepo) CreateTariff(_ context.Context, t domain.Tariff) (domain.Tariff, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextSort++
	t.IsActive = true
	t.SortOrder = r.nextSort
	r.tariffs[t.ID] = t
	return t, nil
}

func (r *fakeCatalogRepo) UpdateTariff(_ context.Context, t domain.Tariff) (domain.Tariff, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.tariffs[t.ID]
	if !ok {
		return domain.Tariff{}, catalog.ErrTariffNotFound
	}
	existing.Name = t.Name
	existing.DiscountPercent = t.DiscountPercent
	existing.Items = t.Items
	r.tariffs[t.ID] = existing
	return existing, nil
}

func (r *fakeCatalogRepo) DeactivateTariff(_ context.Context, id string) (domain.Tariff, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tariffs[id]
	if !ok {
		return domain.Tariff{}, catalog.ErrTariffNotFound
	}
	t.IsActive = false
	r.tariffs[id] = t
	return t, nil
}

type fakeBillingRepo struct {
	mu       sync.Mutex
	credits  map[string]int
	payments map[string]domain.Payment
}

func newFakeBillingRepo() *fakeBillingRepo {
	return &fakeBillingRepo{credits: make(map[string]int), payments: make(map[string]domain.Payment)}
}

func billingCreditKey(sessionID, serviceID string) string { return sessionID + "|" + serviceID }

func (r *fakeBillingRepo) GetBalance(_ context.Context, sessionID string) ([]domain.UserCredit, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.UserCredit, 0, 2)
	for _, serviceID := range []string{"qa", "doc"} {
		out = append(out, domain.UserCredit{ServiceID: serviceID, Quantity: r.credits[billingCreditKey(sessionID, serviceID)]})
	}
	return out, nil
}

func (r *fakeBillingRepo) CreatePayment(_ context.Context, p domain.Payment) (domain.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payments[p.ID] = p
	return p, nil
}

func (r *fakeBillingRepo) GetPaymentByID(_ context.Context, id string) (domain.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.payments[id]
	if !ok {
		return domain.Payment{}, billing.ErrPaymentNotFound
	}
	return p, nil
}

func (r *fakeBillingRepo) ConfirmPayment(_ context.Context, id string, now time.Time) (domain.Payment, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.payments[id]
	if !ok {
		return domain.Payment{}, false, billing.ErrPaymentNotFound
	}
	if p.Status != domain.PaymentStatusPending {
		return p, false, nil
	}
	p.Status = domain.PaymentStatusSuccess
	p.PaidAt = &now
	r.payments[id] = p
	for _, item := range p.Items {
		r.credits[billingCreditKey(p.SessionID, item.ServiceID)] += item.Qty
	}
	return p, true, nil
}

func (r *fakeBillingRepo) FailStalePending(_ context.Context, olderThan time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for id, p := range r.payments {
		if p.Status == domain.PaymentStatusPending && p.CreatedAt.Before(olderThan) {
			p.Status = domain.PaymentStatusFailed
			r.payments[id] = p
			n++
		}
	}
	return n, nil
}

func (r *fakeBillingRepo) DebitCredit(_ context.Context, sessionID, serviceID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := billingCreditKey(sessionID, serviceID)
	if r.credits[key] < 1 {
		return false, nil
	}
	r.credits[key]--
	return true, nil
}

func (r *fakeBillingRepo) CreditBalance(_ context.Context, sessionID, serviceID string, qty int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.credits[billingCreditKey(sessionID, serviceID)] += qty
	return nil
}

// fakeThreadRepo — in-memory реализация thread.Repository для юнит-тестов
// httpserver, без реального Postgres (тот тестируется отдельно,
// internal/repo — testcontainers, internal/repo/thread_repo_test.go).
type fakeThreadRepo struct {
	mu       sync.Mutex
	threads  map[string]domain.Thread
	messages map[string][]domain.Message
}

func newFakeThreadRepo() *fakeThreadRepo {
	return &fakeThreadRepo{
		threads:  make(map[string]domain.Thread),
		messages: make(map[string][]domain.Message),
	}
}

func (r *fakeThreadRepo) CreateThread(_ context.Context, t domain.Thread, firstMessage domain.Message) (domain.Thread, domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	firstMessage.ThreadID = t.ID
	r.threads[t.ID] = t
	r.messages[t.ID] = []domain.Message{firstMessage}
	return t, firstMessage, nil
}

func (r *fakeThreadRepo) GetByID(_ context.Context, id string) (domain.Thread, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.threads[id]
	if !ok {
		return domain.Thread{}, thread.ErrThreadNotFound
	}
	return t, nil
}

func (r *fakeThreadRepo) GetMessages(_ context.Context, threadID string) ([]domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Message, len(r.messages[threadID]))
	copy(out, r.messages[threadID])
	return out, nil
}

func (r *fakeThreadRepo) ListThreads(_ context.Context, sessionID string, filter thread.ListFilter) ([]domain.Thread, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	matched := make([]domain.Thread, 0)
	for _, t := range r.threads {
		if t.SessionID != sessionID || t.IsDeleted() {
			continue
		}
		if filter.Status != nil && t.Status != *filter.Status {
			continue
		}
		if filter.Search != "" &&
			!strings.Contains(strings.ToLower(t.Title), strings.ToLower(filter.Search)) &&
			!strings.Contains(strings.ToLower(t.PreviewText), strings.ToLower(filter.Search)) {
			continue
		}
		matched = append(matched, t)
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].CreatedAt.After(matched[j].CreatedAt) })

	total := len(matched)
	start := min((filter.Page-1)*filter.PageSize, total)
	end := min(start+filter.PageSize, total)
	return matched[start:end], total, nil
}

func (r *fakeThreadRepo) AppendMessage(_ context.Context, msg domain.Message) (domain.Thread, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.threads[msg.ThreadID]
	if !ok {
		return domain.Thread{}, thread.ErrThreadNotFound
	}
	r.messages[msg.ThreadID] = append(r.messages[msg.ThreadID], msg)
	t.MessageCount++
	createdAt := msg.CreatedAt
	t.LastMessageAt = &createdAt
	r.threads[t.ID] = t
	return t, nil
}

func (r *fakeThreadRepo) UpdateStatus(_ context.Context, id string, from []domain.ThreadStatus, to domain.ThreadStatus, previewText string) (domain.Thread, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.threads[id]
	if !ok {
		return domain.Thread{}, false, thread.ErrThreadNotFound
	}
	if !slices.Contains(from, t.Status) {
		return domain.Thread{}, false, nil
	}
	t.Status = to
	t.PreviewText = previewText
	r.threads[id] = t
	return t, true, nil
}

func (r *fakeThreadRepo) ActivatePaid(_ context.Context, id string, paidAt, freeUntil time.Time, previewText string) (domain.Thread, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.threads[id]
	if !ok {
		return domain.Thread{}, false, thread.ErrThreadNotFound
	}
	if t.Status != domain.ThreadStatusQueued || t.IsPaid {
		return domain.Thread{}, false, nil
	}
	t.IsPaid = true
	t.PaidAt = &paidAt
	t.FreeUntil = &freeUntil
	t.Status = domain.ThreadStatusProcessing
	t.PreviewText = previewText
	r.threads[id] = t
	return t, true, nil
}

func (r *fakeThreadRepo) CloseIfExpired(_ context.Context, id string, now time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.threads[id]
	if !ok || t.ClosedAt != nil {
		return false, nil
	}
	t.ClosedAt = &now
	r.threads[id] = t
	return true, nil
}

func (r *fakeThreadRepo) SoftDelete(_ context.Context, id string, now time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.threads[id]
	if !ok || t.DeletedAt != nil {
		return false, nil
	}
	t.DeletedAt = &now
	r.threads[id] = t
	return true, nil
}

func (r *fakeThreadRepo) SetMessageFeedback(_ context.Context, messageID, sessionID string, feedback domain.MessageFeedback) (domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for threadID, msgs := range r.messages {
		t := r.threads[threadID]
		if t.SessionID != sessionID {
			continue
		}
		for i, m := range msgs {
			if m.ID == messageID {
				m.Feedback = &feedback
				msgs[i] = m
				return m, nil
			}
		}
	}
	return domain.Message{}, thread.ErrMessageNotFound
}

// fakePromptRepo — in-memory реализация prompt.Repository (Stage 5),
// предзаполнена так же, как migrations/000005_stage5_agent.up.sql (обе
// записи — qa/document).
type fakePromptRepo struct {
	mu      sync.Mutex
	prompts map[domain.AgentType]domain.AgentPrompt
}

func newFakePromptRepo() *fakePromptRepo {
	return &fakePromptRepo{
		prompts: map[domain.AgentType]domain.AgentPrompt{
			domain.AgentTypeQA:       {ID: "11111111-1111-1111-1111-111111111111", AgentType: domain.AgentTypeQA, PromptText: "test qa prompt"},
			domain.AgentTypeDocument: {ID: "22222222-2222-2222-2222-222222222222", AgentType: domain.AgentTypeDocument, PromptText: "test document prompt"},
		},
	}
}

func (r *fakePromptRepo) GetByAgentType(_ context.Context, agentType domain.AgentType) (domain.AgentPrompt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.prompts[agentType]
	if !ok {
		return domain.AgentPrompt{}, prompt.ErrNotFound
	}
	return p, nil
}

func (r *fakePromptRepo) List(_ context.Context) ([]domain.AgentPrompt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]domain.AgentPrompt, 0, len(r.prompts))
	for _, p := range r.prompts {
		items = append(items, p)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].AgentType < items[j].AgentType })
	return items, nil
}

// fakeDocumentFileStore — in-memory реализация document.FileStore (Stage 6)
// для юнит-тестов httpserver, без реального Postgres (тот тестируется
// отдельно, internal/repo/file_repo_test.go — testcontainers).
type fakeDocumentFileStore struct {
	mu    sync.Mutex
	byKey map[string]domain.FileAttachment
}

func newFakeDocumentFileStore() *fakeDocumentFileStore {
	return &fakeDocumentFileStore{byKey: make(map[string]domain.FileAttachment)}
}

func documentFileStoreKey(threadID, mimeType string) string { return threadID + "|" + mimeType }

func (s *fakeDocumentFileStore) CreateGenerated(_ context.Context, f domain.FileAttachment) (domain.FileAttachment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byKey[documentFileStoreKey(*f.ThreadID, f.MimeType)] = f
	return f, nil
}

func (s *fakeDocumentFileStore) GetLatestGenerated(_ context.Context, threadID, mimeType string) (domain.FileAttachment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.byKey[documentFileStoreKey(threadID, mimeType)]
	if !ok {
		return domain.FileAttachment{}, document.ErrNotGenerated
	}
	return f, nil
}

// fakeDocumentGenerator — document.Generator для юнит-тестов httpserver:
// по умолчанию детерминированный успех (никакого реального internal/agent.Client,
// Stage 5), не задумывается о содержимом истории треда.
type fakeDocumentGenerator struct {
	result document.GenerateResult
	err    error
}

func newFakeDocumentGenerator() *fakeDocumentGenerator {
	return &fakeDocumentGenerator{result: document.GenerateResult{
		AnswerText: "Документ подготовлен.",
		Findings:   []domain.Finding{{Title: "Раздел 1", Body: "Текст раздела."}},
	}}
}

func (g *fakeDocumentGenerator) GenerateDocument(_ context.Context, _ []domain.Message) (document.GenerateResult, error) {
	if g.err != nil {
		return document.GenerateResult{}, g.err
	}
	return g.result, nil
}

// fakeDocumentRenderer — document.Renderer для юнит-тестов httpserver.
// failFormat, если задан, роняет рендер этого формата (частичный успех).
type fakeDocumentRenderer struct {
	failFormat grpcclient.RenderFormat
}

func (r *fakeDocumentRenderer) RenderDocument(_ context.Context, _ string, _ []domain.Finding, format grpcclient.RenderFormat) (grpcclient.RenderResult, error) {
	if r.failFormat != "" && format == r.failFormat {
		return grpcclient.RenderResult{}, errors.New("fakeDocumentRenderer: render failed")
	}
	return grpcclient.RenderResult{
		FileURL:   "https://files.example/generated/doc." + string(format),
		ObjectKey: "generated/doc." + string(format),
	}, nil
}

// fakeDocumentStorage — document.Storage для юнит-тестов httpserver.
type fakeDocumentStorage struct{}

func (fakeDocumentStorage) PresignGetPublic(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://files.example/presigned/" + key, nil
}

func (r *fakePromptRepo) Update(_ context.Context, agentType domain.AgentType, promptText string, updatedAt time.Time) (domain.AgentPrompt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.prompts[agentType]
	if !ok {
		return domain.AgentPrompt{}, prompt.ErrNotFound
	}
	p.PromptText = promptText
	p.UpdatedAt = updatedAt
	r.prompts[agentType] = p
	return p, nil
}

// fakeAnalyticsRepo — in-memory реализация analytics.Repository (Stage 7)
// для юнит-тестов httpserver, без реального Postgres (та же граница, что и
// у остальных fake*Repo в этом файле — реальные агрегаты проверяет
// internal/repo/analytics_repo_test.go, testcontainers). Возвращает
// заранее заданный снимок, не считает его сама — маршрутизация/сериализация
// это всё, что нужно проверить на этом уровне (BACKEND_CODING_STANDARDS.md §10).
type fakeAnalyticsRepo struct {
	overview domain.AnalyticsOverview
}

func newFakeAnalyticsRepo() *fakeAnalyticsRepo {
	return &fakeAnalyticsRepo{}
}

func (r *fakeAnalyticsRepo) GetOverview(context.Context) (domain.AnalyticsOverview, error) {
	return r.overview, nil
}

var _ analytics.Repository = (*fakeAnalyticsRepo)(nil)
