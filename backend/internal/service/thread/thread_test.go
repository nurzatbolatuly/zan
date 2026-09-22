package thread_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/service/billing"
	"zan-backend/internal/service/catalog"
	"zan-backend/internal/service/file"
	"zan-backend/internal/service/thread"
)

// fakeRepo — in-memory реализация thread.Repository для юнит-тестов, без
// БД (BACKEND_CODING_STANDARDS.md §10). Реальная семантика гонок/атомарности
// проверяется отдельно, internal/repo/thread_repo_test.go (testcontainers).
type fakeRepo struct {
	threads  map[string]domain.Thread
	messages map[string][]domain.Message
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{threads: make(map[string]domain.Thread), messages: make(map[string][]domain.Message)}
}

func (r *fakeRepo) CreateThread(_ context.Context, t domain.Thread, firstMessage domain.Message) (domain.Thread, domain.Message, error) {
	firstMessage.ThreadID = t.ID
	r.threads[t.ID] = t
	r.messages[t.ID] = []domain.Message{firstMessage}
	return t, firstMessage, nil
}

func (r *fakeRepo) GetByID(_ context.Context, id string) (domain.Thread, error) {
	t, ok := r.threads[id]
	if !ok {
		return domain.Thread{}, thread.ErrThreadNotFound
	}
	return t, nil
}

func (r *fakeRepo) GetMessages(_ context.Context, threadID string) ([]domain.Message, error) {
	return append([]domain.Message(nil), r.messages[threadID]...), nil
}

func (r *fakeRepo) ListThreads(_ context.Context, sessionID string, filter thread.ListFilter) ([]domain.Thread, int, error) {
	matched := make([]domain.Thread, 0)
	for _, t := range r.threads {
		if t.SessionID == sessionID && !t.IsDeleted() {
			matched = append(matched, t)
		}
	}
	return matched, len(matched), nil
}

func (r *fakeRepo) AppendMessage(_ context.Context, msg domain.Message) (domain.Thread, error) {
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

func (r *fakeRepo) UpdateStatus(_ context.Context, id string, from []domain.ThreadStatus, to domain.ThreadStatus, previewText string) (domain.Thread, bool, error) {
	t, ok := r.threads[id]
	if !ok {
		return domain.Thread{}, false, thread.ErrThreadNotFound
	}
	allowed := false
	for _, f := range from {
		if t.Status == f {
			allowed = true
		}
	}
	if !allowed {
		return domain.Thread{}, false, nil
	}
	t.Status = to
	t.PreviewText = previewText
	r.threads[id] = t
	return t, true, nil
}

func (r *fakeRepo) ActivatePaid(_ context.Context, id string, paidAt, freeUntil time.Time, previewText string) (domain.Thread, bool, error) {
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

func (r *fakeRepo) CloseIfExpired(_ context.Context, id string, now time.Time) (bool, error) {
	t, ok := r.threads[id]
	if !ok || t.ClosedAt != nil {
		return false, nil
	}
	t.ClosedAt = &now
	r.threads[id] = t
	return true, nil
}

func (r *fakeRepo) SoftDelete(_ context.Context, id string, now time.Time) (bool, error) {
	t, ok := r.threads[id]
	if !ok || t.DeletedAt != nil {
		return false, nil
	}
	t.DeletedAt = &now
	r.threads[id] = t
	return true, nil
}

func (r *fakeRepo) SetMessageFeedback(_ context.Context, messageID, sessionID string, feedback domain.MessageFeedback) (domain.Message, error) {
	for threadID, msgs := range r.messages {
		if r.threads[threadID].SessionID != sessionID {
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

// fakeBalance — in-memory реализация thread.BalanceService. DebitCredit
// возвращает именно billing.ErrInsufficientBalance при нулевом балансе —
// тот же сентинел, что и настоящий billing.Service (thread.Service явно
// проверяет errors.Is по нему).
type fakeBalance struct {
	credits     map[string]int
	refunds     []refundCall
	debitBroken bool // симулирует непредвиденную ошибку транспорта (не insufficient)
}

type refundCall struct {
	sessionID, serviceID string
	qty                  int
}

func newFakeBalance() *fakeBalance {
	return &fakeBalance{credits: make(map[string]int)}
}

func balanceKey(sessionID, serviceID string) string { return sessionID + "|" + serviceID }

func (b *fakeBalance) DebitCredit(_ context.Context, sessionID, serviceID string) error {
	if b.debitBroken {
		return errors.New("fakeBalance: broken")
	}
	key := balanceKey(sessionID, serviceID)
	if b.credits[key] < 1 {
		return billing.ErrInsufficientBalance
	}
	b.credits[key]--
	return nil
}

func (b *fakeBalance) RefundCredit(_ context.Context, sessionID, serviceID string, qty int) error {
	b.refunds = append(b.refunds, refundCall{sessionID, serviceID, qty})
	b.credits[balanceKey(sessionID, serviceID)] += qty
	return nil
}

func (b *fakeBalance) GetBalance(_ context.Context, sessionID string) ([]domain.UserCredit, error) {
	out := make([]domain.UserCredit, 0, 2)
	for _, serviceID := range []string{"qa", "doc"} {
		out = append(out, domain.UserCredit{ServiceID: serviceID, Quantity: b.credits[balanceKey(sessionID, serviceID)]})
	}
	return out, nil
}

// fakeCatalog — in-memory реализация thread.ServiceCatalog.
type fakeCatalog struct {
	services map[string]domain.Service
}

func newFakeCatalog() *fakeCatalog {
	return &fakeCatalog{services: map[string]domain.Service{
		"qa":  {ID: "qa", IsActive: true},
		"doc": {ID: "doc", IsActive: true},
	}}
}

func (c *fakeCatalog) GetServiceByID(_ context.Context, id string) (domain.Service, error) {
	s, ok := c.services[id]
	if !ok {
		return domain.Service{}, catalog.ErrServiceNotFound
	}
	return s, nil
}

// fakeFileAttacher — in-memory реализация thread.FileAttacher (Stage 4).
type fakeFileAttacher struct {
	available     map[string]string // fileID -> sessionID, отсутствие ключа = недоступен
	attached      map[string][]string
	validateCalls int
}

func newFakeFileAttacher() *fakeFileAttacher {
	return &fakeFileAttacher{available: make(map[string]string), attached: make(map[string][]string)}
}

func (f *fakeFileAttacher) ValidateAvailable(_ context.Context, sessionID string, fileIDs []string) error {
	f.validateCalls++
	for _, id := range fileIDs {
		if f.available[id] != sessionID {
			return file.ErrNotFound
		}
	}
	return nil
}

func (f *fakeFileAttacher) AttachToMessage(_ context.Context, messageID string, fileIDs []string) error {
	f.attached[messageID] = append(f.attached[messageID], fileIDs...)
	return nil
}

// fakeAgent — thread.Agent с управляемым результатом/ошибкой для теста.
type fakeAgent struct {
	result       thread.AgentResult
	err          error
	lastRequests []thread.AgentRequest
}

func newFakeAgent(result thread.AgentResult) *fakeAgent {
	return &fakeAgent{result: result}
}

func (a *fakeAgent) Process(_ context.Context, req thread.AgentRequest) (thread.AgentResult, error) {
	a.lastRequests = append(a.lastRequests, req)
	if a.err != nil {
		return thread.AgentResult{}, a.err
	}
	return a.result, nil
}

const (
	testSessionID  = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	otherSessionID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
)

type testSetup struct {
	repo    *fakeRepo
	balance *fakeBalance
	catalog *fakeCatalog
	files   *fakeFileAttacher
	agent   *fakeAgent
	svc     *thread.Service
	now     time.Time
}

func newTestSetup(agent *fakeAgent, ids []string) *testSetup {
	repo := newFakeRepo()
	bal := newFakeBalance()
	cat := newFakeCatalog()
	files := newFakeFileAttacher()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := thread.New(repo, bal, cat, files, agent, clock.Fake{T: now}, &idgen.Fake{IDs: ids}, 72*time.Hour)
	return &testSetup{repo: repo, balance: bal, catalog: cat, files: files, agent: agent, svc: svc, now: now}
}

func TestCreateThread_SufficientBalance_ProcessesSynchronouslyToDone(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "Ваш ответ готов.", Sources: []domain.Source{}})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	got, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID,
		Language:  domain.LanguageRu,
		ServiceID: "qa",
		Text:      "Как оформить развод?",
		InputType: domain.MessageInputTypeText,
	})

	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusDone, got.Status)
	require.True(t, got.IsPaid)
	require.NotNil(t, got.PaidAt)
	require.NotNil(t, got.FreeUntil)
	require.Equal(t, "Как оформить развод?", got.Title)
	require.Equal(t, "Ваш ответ готов.", got.PreviewText)
	require.Equal(t, 2, got.MessageCount) // пользователь + ассистент
	require.Len(t, setup.agent.lastRequests, 1)

	msgs, err := setup.repo.GetMessages(context.Background(), got.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	require.Equal(t, domain.MessageSenderAssistant, msgs[1].Sender)
	require.Equal(t, "Ваш ответ готов.", msgs[1].Text)

	balance, _ := setup.balance.GetBalance(context.Background(), testSessionID)
	require.Contains(t, balance, domain.UserCredit{ServiceID: "qa", Quantity: 0})
}

// steppingClock — clock.Clock, увеличивающий время на step при каждом
// вызове Now() — нужен только тесту ProcessingTimeMs ниже (Stage 7):
// newTestSetup использует фиксированный clock.Fake (не должен меняться для
// остальных тестов, которые полагаются на конкретные timestamp'ы), здесь
// нужен счётчик прошедшего "времени" без реального time.Sleep
// (BACKEND_CODING_STANDARDS.md §10).
type steppingClock struct {
	t    time.Time
	step time.Duration
}

func (c *steppingClock) Now() time.Time {
	now := c.t
	c.t = c.t.Add(c.step)
	return now
}

func TestCreateThread_SetsProcessingTimeMsOnAssistantMessage(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "Ответ"})
	repo := newFakeRepo()
	bal := newFakeBalance()
	bal.credits[balanceKey(testSessionID, "qa")] = 1
	clk := &steppingClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), step: 250 * time.Millisecond}
	svc := thread.New(repo, bal, newFakeCatalog(), newFakeFileAttacher(), agent, clk, &idgen.Fake{IDs: []string{"thread-1", "msg-user-1", "msg-assistant-1"}}, 72*time.Hour)

	got, err := svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, Language: domain.LanguageRu, ServiceID: "qa", Text: "Вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)

	msgs, err := repo.GetMessages(context.Background(), got.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	require.Nil(t, msgs[0].ProcessingTimeMs, "user message has no processing time")
	require.NotNil(t, msgs[1].ProcessingTimeMs, "assistant message must carry processing_time_ms (Stage 7 analytics)")
	require.Positive(t, *msgs[1].ProcessingTimeMs)
}

func TestCreateThread_UnverifiedSources_PropagatesToSavedMessage(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{
		Status:            thread.AgentResultDone,
		AnswerText:        "Ответ со спорным источником.",
		Sources:           []domain.Source{{Ref: "ст. 999 Придуманного кодекса", Quote: "x"}},
		UnverifiedSources: true,
	})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	got, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID,
		Language:  domain.LanguageRu,
		ServiceID: "qa",
		Text:      "Вопрос",
		InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)

	msgs, err := setup.repo.GetMessages(context.Background(), got.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	require.False(t, msgs[0].UnverifiedSources, "user message is never unverified")
	require.True(t, msgs[1].UnverifiedSources)
}

func TestCreateThread_InsufficientBalance_CreatesQueuedUnpaid(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "не должно вызваться"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1"})

	got, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID,
		Language:  domain.LanguageRu,
		ServiceID: "qa",
		Text:      "Вопрос без баланса",
		InputType: domain.MessageInputTypeText,
	})

	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusQueued, got.Status)
	require.False(t, got.IsPaid)
	require.Nil(t, got.PaidAt)
	require.Nil(t, got.FreeUntil)
	require.Empty(t, setup.agent.lastRequests, "агент не должен вызываться для неоплаченного треда")
}

func TestCreateThread_UnknownService_ReturnsServiceNotFound(t *testing.T) {
	setup := newTestSetup(newFakeAgent(thread.AgentResult{}), nil)

	_, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID,
		ServiceID: "unknown",
		Text:      "текст",
		InputType: domain.MessageInputTypeText,
	})

	require.ErrorIs(t, err, catalog.ErrServiceNotFound)
}

func TestCreateThread_RejectsEmptyText(t *testing.T) {
	setup := newTestSetup(newFakeAgent(thread.AgentResult{}), nil)

	_, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID,
		ServiceID: "qa",
		Text:      "   ",
		InputType: domain.MessageInputTypeText,
	})

	require.ErrorIs(t, err, thread.ErrEmptyText)
}

// TestCreateThread_RejectsUnsupportedInputType — thread.Service — последний
// рубеж защиты от значения domain.MessageInputType, которое не прошло бы
// domain.ParseMessageInputType (httpserver отклонил бы его раньше, 400 —
// см. thread.go writeThreadError); тест конструирует такое значение
// напрямую, минуя парсинг, ровно затем, чтобы проверить именно этот рубеж.
func TestCreateThread_RejectsUnsupportedInputType(t *testing.T) {
	setup := newTestSetup(newFakeAgent(thread.AgentResult{}), nil)

	_, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID,
		ServiceID: "qa",
		Text:      "текст",
		InputType: domain.MessageInputType("bogus"),
	})

	require.ErrorIs(t, err, thread.ErrUnsupportedInputType)
}

// TestCreateThread_Voice_AcceptsAlreadyTranscribedText — Stage 4:
// input_type=voice больше не отклоняется — к моменту вызова POST /threads
// текст уже расшифрован клиентом через POST /voice/transcribe
// (zan-backend-tz-v2.md §4.7), thread.Service обращается с ним как с
// обычным текстом.
func TestCreateThread_Voice_AcceptsAlreadyTranscribedText(t *testing.T) {
	setup := newTestSetup(newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "ответ"}), []string{"thread-1", "msg-user-1", "msg-assistant-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	got, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID,
		ServiceID: "qa",
		Text:      "уже расшифрованный текст",
		InputType: domain.MessageInputTypeVoice,
	})

	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusDone, got.Status)
}

// TestCreateThread_File_RequiresEitherTextOrFiles — input_type=file без
// текста и без file_ids не несёт никакого содержания.
func TestCreateThread_File_RequiresEitherTextOrFiles(t *testing.T) {
	setup := newTestSetup(newFakeAgent(thread.AgentResult{}), nil)

	_, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID,
		ServiceID: "qa",
		InputType: domain.MessageInputTypeFile,
	})

	require.ErrorIs(t, err, thread.ErrEmptyText)
}

// TestCreateThread_File_RejectsFileFromOtherSessionOrAlreadyAttached —
// FileAttacher.ValidateAvailable решает, доступен ли file_id, thread.Service
// пробрасывает его ошибку как есть (httpserver мапит на file_not_found).
func TestCreateThread_File_RejectsUnavailableFile(t *testing.T) {
	setup := newTestSetup(newFakeAgent(thread.AgentResult{}), nil)

	_, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID,
		ServiceID: "qa",
		InputType: domain.MessageInputTypeFile,
		FileIDs:   []string{"file-not-owned"},
	})

	require.ErrorIs(t, err, file.ErrNotFound)
	require.Equal(t, 1, setup.files.validateCalls)
}

// TestCreateThread_File_AttachesFilesAfterMessageCreated — сообщение без
// текста, только с вложением, успешно создаёт тред и привязывает файл к
// первому сообщению.
func TestCreateThread_File_AttachesFilesAfterMessageCreated(t *testing.T) {
	setup := newTestSetup(newFakeAgent(thread.AgentResult{}), []string{"thread-1", "msg-user-1"})
	setup.files.available["file-1"] = testSessionID

	got, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID,
		ServiceID: "qa",
		InputType: domain.MessageInputTypeFile,
		FileIDs:   []string{"file-1"},
	})

	require.NoError(t, err)
	msgs, err := setup.repo.GetMessages(context.Background(), got.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, []string{"file-1"}, setup.files.attached[msgs[0].ID])
}

func TestCreateThread_AgentError_MarksErrorAndRefundsCredit(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultError})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	got, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID,
		ServiceID: "qa",
		Text:      "вопрос",
		InputType: domain.MessageInputTypeText,
	})

	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusError, got.Status)
	require.Len(t, setup.balance.refunds, 1)
	require.Equal(t, refundCall{testSessionID, "qa", 1}, setup.balance.refunds[0])
	balance, _ := setup.balance.GetBalance(context.Background(), testSessionID)
	require.Contains(t, balance, domain.UserCredit{ServiceID: "qa", Quantity: 1}) // списали и тут же вернули
}

func TestAddMessage_WithinFreeUntil_ContinuesThreadWithoutNewDebit(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "первый ответ"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1", "msg-user-2", "msg-assistant-2"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос 1", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusDone, created.Status)

	agent.result = thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "уточнение готово"}
	updated, err := setup.svc.AddMessage(context.Background(), thread.AddMessageRequest{
		ThreadID: created.ID, SessionID: testSessionID, Text: "уточняющий вопрос", InputType: domain.MessageInputTypeText,
	})

	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusDone, updated.Status)
	require.Equal(t, 4, updated.MessageCount)
	require.Equal(t, "уточнение готово", updated.PreviewText)

	// Баланс не списан повторно — бесплатное уточнение (§4.3).
	balance, _ := setup.balance.GetBalance(context.Background(), testSessionID)
	require.Contains(t, balance, domain.UserCredit{ServiceID: "qa", Quantity: 0})
}

// TestAddMessage_WithFileIDs_AttachesToNewMessage — уточнение с вложением
// (input_type=file, без текста) внутри уже открытого треда (Stage 4).
func TestAddMessage_WithFileIDs_AttachesToNewMessage(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "первый ответ"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1", "msg-user-2", "msg-assistant-2"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос 1", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)

	setup.files.available["file-1"] = testSessionID
	_, err = setup.svc.AddMessage(context.Background(), thread.AddMessageRequest{
		ThreadID: created.ID, SessionID: testSessionID, InputType: domain.MessageInputTypeFile, FileIDs: []string{"file-1"},
	})
	require.NoError(t, err)

	msgs, err := setup.repo.GetMessages(context.Background(), created.ID)
	require.NoError(t, err)
	secondUserMsg := msgs[2] // [user-1, assistant-1, user-2(file)]
	require.Equal(t, domain.MessageInputTypeFile, secondUserMsg.InputType)
	require.Equal(t, []string{"file-1"}, setup.files.attached[secondUserMsg.ID])
}

func TestAddMessage_AfterFreeUntilExpired_ReturnsClosedAndClosesThread(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "первый ответ"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)

	future := setup.now.Add(73 * time.Hour) // free_until = now + 72h
	svcLater := thread.New(setup.repo, setup.balance, setup.catalog, setup.files, agent, clock.Fake{T: future}, &idgen.Fake{}, 72*time.Hour)

	_, err = svcLater.AddMessage(context.Background(), thread.AddMessageRequest{
		ThreadID: created.ID, SessionID: testSessionID, Text: "поздно", InputType: domain.MessageInputTypeText,
	})
	require.ErrorIs(t, err, thread.ErrThreadClosed)

	closedThread, closeErr := setup.repo.GetByID(context.Background(), created.ID)
	require.NoError(t, closeErr)
	require.NotNil(t, closedThread.ClosedAt)
}

func TestAddMessage_OnQueuedUnpaidThread_ReturnsPaymentRequired(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1"})

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "без баланса", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusQueued, created.Status)

	_, err = setup.svc.AddMessage(context.Background(), thread.AddMessageRequest{
		ThreadID: created.ID, SessionID: testSessionID, Text: "ещё", InputType: domain.MessageInputTypeText,
	})
	require.ErrorIs(t, err, thread.ErrPaymentRequired)
}

func TestAddMessage_OnErrorThread_ReturnsNotActive(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultError})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusError, created.Status)

	_, err = setup.svc.AddMessage(context.Background(), thread.AddMessageRequest{
		ThreadID: created.ID, SessionID: testSessionID, Text: "ещё раз", InputType: domain.MessageInputTypeText,
	})
	require.ErrorIs(t, err, thread.ErrThreadNotActive)
}

func TestAddMessage_ForeignSession_ReturnsNotFound(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "ответ"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)

	_, err = setup.svc.AddMessage(context.Background(), thread.AddMessageRequest{
		ThreadID: created.ID, SessionID: otherSessionID, Text: "чужой вопрос", InputType: domain.MessageInputTypeText,
	})
	require.ErrorIs(t, err, thread.ErrThreadNotFound)
}

func TestActivateAfterPayment_ActivatesQueuedThreadAndProcesses(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "готово после оплаты"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1"})

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос без баланса", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusQueued, created.Status)

	// Баланс появился (эмуляция billing.ConfirmPayment{thread_id}).
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	activated, err := setup.svc.ActivateAfterPayment(context.Background(), created.ID, testSessionID, domain.LanguageRu)
	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusDone, activated.Status)
	require.True(t, activated.IsPaid)
	require.NotNil(t, activated.FreeUntil)
}

func TestActivateAfterPayment_AlreadyActivated_IsIdempotent(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "готово"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	require.True(t, created.IsPaid)

	again, err := setup.svc.ActivateAfterPayment(context.Background(), created.ID, testSessionID, domain.LanguageRu)
	require.NoError(t, err)
	require.Equal(t, created.Status, again.Status)
	require.Len(t, agent.lastRequests, 1, "повторная активация не должна снова вызывать агента")
}

func TestCancelThread_FromQueued_Succeeds(t *testing.T) {
	setup := newTestSetup(newFakeAgent(thread.AgentResult{}), []string{"thread-1", "msg-user-1"})

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)

	canceled, err := setup.svc.CancelThread(context.Background(), created.ID, testSessionID)
	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusCanceled, canceled.Status)
}

func TestCancelThread_FromDone_Rejected(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "ответ"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusDone, created.Status)

	_, err = setup.svc.CancelThread(context.Background(), created.ID, testSessionID)
	require.ErrorIs(t, err, thread.ErrCannotCancel)
}

func TestDeleteThread_SoftDeletes_ThenNotFound(t *testing.T) {
	setup := newTestSetup(newFakeAgent(thread.AgentResult{}), []string{"thread-1", "msg-user-1"})

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)

	require.NoError(t, setup.svc.DeleteThread(context.Background(), created.ID, testSessionID))

	_, _, err = setup.svc.GetThread(context.Background(), created.ID, testSessionID)
	require.ErrorIs(t, err, thread.ErrThreadNotFound)

	err = setup.svc.DeleteThread(context.Background(), created.ID, testSessionID)
	require.ErrorIs(t, err, thread.ErrThreadNotFound, "повторное удаление — тот же not_found, не паника")
}

func TestSetMessageFeedback_UpdatesMessage(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "ответ"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)

	_, msgs, err := setup.svc.GetThread(context.Background(), created.ID, testSessionID)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assistantMsgID := msgs[1].ID

	updated, err := setup.svc.SetMessageFeedback(context.Background(), assistantMsgID, testSessionID, domain.MessageFeedbackLike)
	require.NoError(t, err)
	require.NotNil(t, updated.Feedback)
	require.Equal(t, domain.MessageFeedbackLike, *updated.Feedback)
}
