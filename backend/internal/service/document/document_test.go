package document_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/grpcclient"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/service/billing"
	"zan-backend/internal/service/catalog"
	"zan-backend/internal/service/document"
)

// fakeThreadStore — in-memory реализация document.ThreadStore, без БД
// (BACKEND_CODING_STANDARDS.md §10). Реальная семантика — internal/repo/
// file_repo_test.go, internal/repo/thread_repo_test.go (testcontainers).
type fakeThreadStore struct {
	threads   map[string]domain.Thread
	messages  map[string][]domain.Message
	appendErr error
}

func newFakeThreadStore() *fakeThreadStore {
	return &fakeThreadStore{threads: make(map[string]domain.Thread), messages: make(map[string][]domain.Message)}
}

func (s *fakeThreadStore) GetByID(_ context.Context, id string) (domain.Thread, error) {
	t, ok := s.threads[id]
	if !ok {
		return domain.Thread{}, document.ErrThreadNotFound
	}
	return t, nil
}

func (s *fakeThreadStore) GetMessages(_ context.Context, threadID string) ([]domain.Message, error) {
	return append([]domain.Message(nil), s.messages[threadID]...), nil
}

func (s *fakeThreadStore) AppendMessage(_ context.Context, msg domain.Message) (domain.Thread, error) {
	if s.appendErr != nil {
		return domain.Thread{}, s.appendErr
	}
	t, ok := s.threads[msg.ThreadID]
	if !ok {
		return domain.Thread{}, document.ErrThreadNotFound
	}
	s.messages[msg.ThreadID] = append(s.messages[msg.ThreadID], msg)
	t.MessageCount++
	s.threads[t.ID] = t
	return t, nil
}

// fakeFileStore — in-memory реализация document.FileStore.
type fakeFileStore struct {
	byThreadAndMime map[string]domain.FileAttachment
	createErr       error
	created         []domain.FileAttachment
}

func newFakeFileStore() *fakeFileStore {
	return &fakeFileStore{byThreadAndMime: make(map[string]domain.FileAttachment)}
}

func fileStoreKey(threadID, mimeType string) string { return threadID + "|" + mimeType }

func (s *fakeFileStore) CreateGenerated(_ context.Context, f domain.FileAttachment) (domain.FileAttachment, error) {
	if s.createErr != nil {
		return domain.FileAttachment{}, s.createErr
	}
	s.created = append(s.created, f)
	s.byThreadAndMime[fileStoreKey(*f.ThreadID, f.MimeType)] = f
	return f, nil
}

func (s *fakeFileStore) GetLatestGenerated(_ context.Context, threadID, mimeType string) (domain.FileAttachment, error) {
	f, ok := s.byThreadAndMime[fileStoreKey(threadID, mimeType)]
	if !ok {
		return domain.FileAttachment{}, errors.New("fakeFileStore: not found")
	}
	return f, nil
}

// fakeBalance — in-memory реализация document.BalanceService. Тот же
// сентинел billing.ErrInsufficientBalance, что и настоящий billing.Service
// (document.Service явно проверяет errors.Is по нему через httpserver, не
// сам Service — см. internal/httpserver/document.go).
type fakeBalance struct {
	credits map[string]int
	refunds []refundCall
}

type refundCall struct {
	sessionID, serviceID string
	qty                  int
}

func newFakeBalance() *fakeBalance {
	return &fakeBalance{credits: make(map[string]int)}
}

func (b *fakeBalance) DebitCredit(_ context.Context, sessionID, serviceID string) error {
	key := sessionID + "|" + serviceID
	if b.credits[key] < 1 {
		return billing.ErrInsufficientBalance
	}
	b.credits[key]--
	return nil
}

func (b *fakeBalance) RefundCredit(_ context.Context, sessionID, serviceID string, qty int) error {
	b.refunds = append(b.refunds, refundCall{sessionID, serviceID, qty})
	b.credits[sessionID+"|"+serviceID] += qty
	return nil
}

// fakeCatalog — in-memory реализация document.ServiceCatalog.
type fakeCatalog struct{}

func (fakeCatalog) GetServiceByID(_ context.Context, id string) (domain.Service, error) {
	if id != "doc" {
		return domain.Service{}, catalog.ErrServiceNotFound
	}
	return domain.Service{ID: "doc", IsActive: true}, nil
}

// fakeGenerator — in-memory реализация document.Generator.
type fakeGenerator struct {
	result document.GenerateResult
	err    error
}

func (g *fakeGenerator) GenerateDocument(_ context.Context, _ []domain.Message) (document.GenerateResult, error) {
	if g.err != nil {
		return document.GenerateResult{}, g.err
	}
	return g.result, nil
}

// fakeRenderer — in-memory реализация document.Renderer. failFormat, если
// задан, роняет рендер именно этого формата (частичный успех).
type fakeRenderer struct {
	failFormat grpcclient.RenderFormat
	calls      []grpcclient.RenderFormat
}

func (r *fakeRenderer) RenderDocument(_ context.Context, _ string, _ []domain.Finding, format grpcclient.RenderFormat) (grpcclient.RenderResult, error) {
	r.calls = append(r.calls, format)
	if r.failFormat != "" && format == r.failFormat {
		return grpcclient.RenderResult{}, errors.New("fakeRenderer: render failed")
	}
	return grpcclient.RenderResult{FileURL: "https://files.example/" + string(format), ObjectKey: "generated/doc." + string(format)}, nil
}

// fakeStorage — in-memory реализация document.Storage.
type fakeStorage struct{}

func (fakeStorage) PresignGetPublic(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://files.example/presigned/" + key, nil
}

const (
	testSessionID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	testThreadID  = "11111111-1111-1111-1111-111111111111"
)

type testSetup struct {
	threads   *fakeThreadStore
	files     *fakeFileStore
	balance   *fakeBalance
	generator *fakeGenerator
	renderer  *fakeRenderer
	svc       *document.Service
}

func newTestSetup(generator *fakeGenerator, renderer *fakeRenderer, ids []string) *testSetup {
	threads := newFakeThreadStore()
	threads.threads[testThreadID] = domain.Thread{
		ID: testThreadID, SessionID: testSessionID, ServiceID: "qa",
		Status: domain.ThreadStatusDone, Title: "Как оформить развод?",
	}
	files := newFakeFileStore()
	bal := newFakeBalance()
	bal.credits[testSessionID+"|doc"] = 1

	svc := document.New(threads, files, bal, fakeCatalog{}, generator, renderer, fakeStorage{},
		clock.Fake{T: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}, &idgen.Fake{IDs: ids})
	return &testSetup{threads: threads, files: files, balance: bal, generator: generator, renderer: renderer, svc: svc}
}

func TestGenerate_HappyPath_SavesMessageAndRendersBothFormats(t *testing.T) {
	generator := &fakeGenerator{result: document.GenerateResult{
		AnswerText: "Документ подготовлен.",
		Findings:   []domain.Finding{{Title: "Стороны", Body: "..."}},
	}}
	renderer := &fakeRenderer{}
	setup := newTestSetup(generator, renderer, []string{"msg-1", "file-pdf", "file-docx"})

	msg, filesReady, err := setup.svc.Generate(context.Background(), testThreadID, testSessionID)

	require.NoError(t, err)
	require.True(t, filesReady)
	require.Equal(t, "Документ подготовлен.", msg.Text)
	require.Equal(t, domain.MessageSenderAssistant, msg.Sender)
	require.Len(t, setup.renderer.calls, 2)
	require.Equal(t, 0, setup.balance.credits[testSessionID+"|doc"])
	require.Empty(t, setup.balance.refunds)

	pdf, err := setup.files.GetLatestGenerated(context.Background(), testThreadID, "application/pdf")
	require.NoError(t, err)
	require.Equal(t, domain.FilePurposeGeneratedOutput, pdf.Purpose)
	require.Equal(t, []string{"pdf", "docx"}, pdf.OutputFormats)
	require.Equal(t, msg.ID, *pdf.MessageID)
	require.Equal(t, testThreadID, *pdf.ThreadID)

	docx, err := setup.files.GetLatestGenerated(context.Background(), testThreadID, "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	require.NoError(t, err)
	require.Equal(t, "generated/doc.docx", docx.ObjectKey)
}

// steppingClock — clock.Clock, увеличивающий время на step при каждом
// вызове Now() — нужен только тесту ProcessingTimeMs ниже (Stage 7):
// newTestSetup использует фиксированный clock.Fake (не должен меняться для
// остальных тестов), здесь нужен счётчик прошедшего "времени" без
// реального time.Sleep (BACKEND_CODING_STANDARDS.md §10, тот же приём, что
// internal/service/thread/thread_test.go — отдельная копия, т.к. пакеты
// _test разные, тип неэкспортируемый).
type steppingClock struct {
	t    time.Time
	step time.Duration
}

func (c *steppingClock) Now() time.Time {
	now := c.t
	c.t = c.t.Add(c.step)
	return now
}

func TestGenerate_SetsProcessingTimeMsOnAssistantMessage(t *testing.T) {
	threads := newFakeThreadStore()
	threads.threads[testThreadID] = domain.Thread{
		ID: testThreadID, SessionID: testSessionID, ServiceID: "qa",
		Status: domain.ThreadStatusDone, Title: "Как оформить развод?",
	}
	bal := newFakeBalance()
	bal.credits[testSessionID+"|doc"] = 1
	generator := &fakeGenerator{result: document.GenerateResult{
		AnswerText: "Документ подготовлен.",
		Findings:   []domain.Finding{{Title: "A", Body: "B"}},
	}}
	clk := &steppingClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), step: 300 * time.Millisecond}
	svc := document.New(threads, newFakeFileStore(), bal, fakeCatalog{}, generator, &fakeRenderer{}, fakeStorage{},
		clk, &idgen.Fake{IDs: []string{"msg-1", "file-pdf", "file-docx"}})

	msg, filesReady, err := svc.Generate(context.Background(), testThreadID, testSessionID)

	require.NoError(t, err)
	require.True(t, filesReady)
	require.NotNil(t, msg.ProcessingTimeMs, "assistant message must carry processing_time_ms (Stage 7 analytics)")
	require.Positive(t, *msg.ProcessingTimeMs)
}

func TestGenerate_InsufficientBalance_ReturnsErrInsufficientBalanceWithoutCallingLLM(t *testing.T) {
	generator := &fakeGenerator{}
	renderer := &fakeRenderer{}
	setup := newTestSetup(generator, renderer, nil)
	setup.balance.credits[testSessionID+"|doc"] = 0

	_, _, err := setup.svc.Generate(context.Background(), testThreadID, testSessionID)

	require.ErrorIs(t, err, billing.ErrInsufficientBalance)
}

func TestGenerate_UnknownThread_ReturnsErrThreadNotFound(t *testing.T) {
	setup := newTestSetup(&fakeGenerator{}, &fakeRenderer{}, nil)

	_, _, err := setup.svc.Generate(context.Background(), "99999999-9999-9999-9999-999999999999", testSessionID)

	require.ErrorIs(t, err, document.ErrThreadNotFound)
}

func TestGenerate_ForeignSession_ReturnsErrThreadNotFound(t *testing.T) {
	setup := newTestSetup(&fakeGenerator{}, &fakeRenderer{}, nil)

	_, _, err := setup.svc.Generate(context.Background(), testThreadID, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")

	require.ErrorIs(t, err, document.ErrThreadNotFound)
}

func TestGenerate_LLMFails_RefundsCreditAndSavesNoMessage(t *testing.T) {
	generator := &fakeGenerator{err: errors.New("agent: openai http 401")}
	setup := newTestSetup(generator, &fakeRenderer{}, nil)

	_, _, err := setup.svc.Generate(context.Background(), testThreadID, testSessionID)

	require.ErrorIs(t, err, document.ErrGenerationFailed)
	require.Equal(t, 1, setup.balance.credits[testSessionID+"|doc"], "credit refunded")
	require.Len(t, setup.balance.refunds, 1)
	require.Equal(t, "doc", setup.balance.refunds[0].serviceID)
	msgs, _ := setup.threads.GetMessages(context.Background(), testThreadID)
	require.Empty(t, msgs)
}

func TestGenerate_RenderFails_PreservesSavedMessageAndRefundsCredit(t *testing.T) {
	generator := &fakeGenerator{result: document.GenerateResult{AnswerText: "Ответ готов.", Findings: []domain.Finding{{Title: "A", Body: "B"}}}}
	renderer := &fakeRenderer{failFormat: grpcclient.RenderFormatDOCX}
	setup := newTestSetup(generator, renderer, []string{"msg-1", "file-pdf"})

	msg, filesReady, err := setup.svc.Generate(context.Background(), testThreadID, testSessionID)

	require.NoError(t, err, "render failure is not a transport error — the saved message is a valid result")
	require.False(t, filesReady)
	require.Equal(t, "Ответ готов.", msg.Text)
	require.Equal(t, 1, setup.balance.credits[testSessionID+"|doc"], "credit refunded even though message was saved")
	require.Len(t, setup.balance.refunds, 1)

	msgs, _ := setup.threads.GetMessages(context.Background(), testThreadID)
	require.Len(t, msgs, 1, "message is preserved despite the render failure")

	// PDF (первый в порядке рендера) успел сохраниться до сбоя DOCX — не откатывается.
	_, err = setup.files.GetLatestGenerated(context.Background(), testThreadID, "application/pdf")
	require.NoError(t, err)
}

func TestGetDocument_ReturnsLatestGeneratedFileWithFreshURL(t *testing.T) {
	setup := newTestSetup(&fakeGenerator{}, &fakeRenderer{}, nil)
	setup.files.byThreadAndMime[fileStoreKey(testThreadID, "application/pdf")] = domain.FileAttachment{
		ID: "file-1", SessionID: testSessionID, ObjectKey: "generated/doc.pdf", MimeType: "application/pdf",
	}

	f, url, err := setup.svc.GetDocument(context.Background(), testThreadID, testSessionID, "pdf")

	require.NoError(t, err)
	require.Equal(t, "file-1", f.ID)
	require.Equal(t, "https://files.example/presigned/generated/doc.pdf", url)
}

func TestGetDocument_NotGeneratedYet_ReturnsErrNotGenerated(t *testing.T) {
	setup := newTestSetup(&fakeGenerator{}, &fakeRenderer{}, nil)

	_, _, err := setup.svc.GetDocument(context.Background(), testThreadID, testSessionID, "pdf")

	require.ErrorIs(t, err, document.ErrNotGenerated)
}

func TestGetDocument_InvalidFormat_ReturnsErrInvalidFormat(t *testing.T) {
	setup := newTestSetup(&fakeGenerator{}, &fakeRenderer{}, nil)

	_, _, err := setup.svc.GetDocument(context.Background(), testThreadID, testSessionID, "txt")

	require.ErrorIs(t, err, document.ErrInvalidFormat)
}

func TestGetDocument_ForeignSession_ReturnsErrThreadNotFound(t *testing.T) {
	setup := newTestSetup(&fakeGenerator{}, &fakeRenderer{}, nil)

	_, _, err := setup.svc.GetDocument(context.Background(), testThreadID, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "pdf")

	require.ErrorIs(t, err, document.ErrThreadNotFound)
}
