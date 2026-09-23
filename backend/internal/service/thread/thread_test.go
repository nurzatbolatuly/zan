package thread_test

import (
	"context"
	"errors"
	"sync"
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
// Stage 9: обработка фоновая (dispatchProcessing) — репозиторий теперь
// читается/пишется из горутины теста И из фоновой горутины конкурентно,
// mu обязателен (иначе -race валится на конкурентном доступе к map).
type fakeRepo struct {
	mu       sync.Mutex
	threads  map[string]domain.Thread
	messages map[string][]domain.Message
	// activateLosesRace — симулирует параллельный запрос, успевший
	// запустить вопрос между списанием и переходом
	// AwaitingPayment -> Processing (UpdateStatus ok=false).
	activateLosesRace bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{threads: make(map[string]domain.Thread), messages: make(map[string][]domain.Message)}
}

func (r *fakeRepo) CreateThread(_ context.Context, t domain.Thread, firstMessage domain.Message) (domain.Thread, domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	firstMessage.ThreadID = t.ID
	r.threads[t.ID] = t
	r.messages[t.ID] = []domain.Message{firstMessage}
	return t, firstMessage, nil
}

func (r *fakeRepo) GetByID(_ context.Context, id string) (domain.Thread, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.threads[id]
	if !ok {
		return domain.Thread{}, thread.ErrThreadNotFound
	}
	return t, nil
}

func (r *fakeRepo) GetMessages(_ context.Context, threadID string) ([]domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.Message(nil), r.messages[threadID]...), nil
}

func (r *fakeRepo) GetConversation(ctx context.Context, threadID string) ([]domain.Message, error) {
	return r.GetMessages(ctx, threadID)
}

func (r *fakeRepo) ListThreads(_ context.Context, sessionID string, filter thread.ListFilter) ([]domain.Thread, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	matched := make([]domain.Thread, 0)
	for _, t := range r.threads {
		if t.SessionID == sessionID && !t.IsDeleted() {
			matched = append(matched, t)
		}
	}
	return matched, len(matched), nil
}

func (r *fakeRepo) AppendMessage(_ context.Context, msg domain.Message) (domain.Thread, error) {
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

func (r *fakeRepo) UpdateStatus(_ context.Context, id string, from []domain.ThreadStatus, to domain.ThreadStatus, previewText string) (domain.Thread, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
	if !allowed || (r.activateLosesRace && to == domain.ThreadStatusProcessing) {
		return domain.Thread{}, false, nil
	}
	t.Status = to
	t.PreviewText = previewText
	r.threads[id] = t
	return t, true, nil
}

func (r *fakeRepo) SoftDelete(_ context.Context, id string, now time.Time) (bool, error) {
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

func (r *fakeRepo) SetMessageFeedback(_ context.Context, messageID, sessionID string, feedback domain.MessageFeedback) (domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
// проверяет errors.Is по нему). Stage 9: RefundCredit теперь может
// вызываться из фоновой горутины (finishWithError) конкурентно с чтением
// из горутины теста — mu обязателен, та же причина, что у fakeRepo.
type fakeBalance struct {
	mu          sync.Mutex
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
	b.mu.Lock()
	defer b.mu.Unlock()
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
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refunds = append(b.refunds, refundCall{sessionID, serviceID, qty})
	b.credits[balanceKey(sessionID, serviceID)] += qty
	return nil
}

func (b *fakeBalance) GetBalance(_ context.Context, sessionID string) ([]domain.UserCredit, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]domain.UserCredit, 0, 2)
	for _, serviceID := range []string{"qa", "doc"} {
		out = append(out, domain.UserCredit{ServiceID: serviceID, Quantity: b.credits[balanceKey(sessionID, serviceID)]})
	}
	return out, nil
}

// refundsSnapshot — доступ к refunds под mu (тесты читают его напрямую
// после ожидания терминального статуса — waitForTerminal синхронизируется
// через fakeRepo, не fakeBalance, поэтому чтение поля всё равно должно
// идти через метод, а не напрямую).
func (b *fakeBalance) refundsSnapshot() []refundCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]refundCall(nil), b.refunds...)
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
// Stage 9: обработка теперь фоновая (dispatchProcessing) — Process
// вызывается из горутины, отдельной от той, что читает/пишет result
// (например, строка 549 переустанавливает result между CreateThread и
// AddMessage) и от той, что проверяет lastRequests — result/lastRequests
// под mu, доступ только через методы ниже (BACKEND_CODING_STANDARDS.md
// §10 — тесты тоже не должны гонять данные под -race).
type fakeAgent struct {
	mu           sync.Mutex
	result       thread.AgentResult
	err          error
	lastRequests []thread.AgentRequest

	// deltas — если непусто, каждый элемент передаётся req.OnDelta по
	// порядку перед тем, как Process вернёт результат (эмуляция Фазы 1
	// streaming-вызова) — настраивается ДО того, как Process может быть
	// вызван конкурентно (до CreateThread/AddMessage), поэтому без mu.
	deltas []string
	// block — если не nil, Process ждёт закрытия канала перед тем, как
	// продолжить — тест "CreateThread возвращается до завершения агента"
	// использует это, чтобы детерминированно застать тред в Processing.
	// Настраивается до вызова CreateThread/AddMessage, без mu по той же
	// причине, что deltas.
	block chan struct{}
	// panicWith — если не nil, Process паникует этим значением вместо
	// обычного возврата (тест panic-recovery в dispatchProcessing).
	// Настраивается до вызова CreateThread/AddMessage, без mu.
	panicWith any
}

func newFakeAgent(result thread.AgentResult) *fakeAgent {
	return &fakeAgent{result: result}
}

func (a *fakeAgent) setResult(result thread.AgentResult) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.result = result
}

func (a *fakeAgent) requestCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.lastRequests)
}

func (a *fakeAgent) Process(_ context.Context, req thread.AgentRequest) (thread.AgentResult, error) {
	a.mu.Lock()
	a.lastRequests = append(a.lastRequests, req)
	result, err := a.result, a.err
	a.mu.Unlock()

	if a.block != nil {
		<-a.block
	}
	if a.panicWith != nil {
		panic(a.panicWith)
	}
	if req.OnDelta != nil {
		for _, d := range a.deltas {
			req.OnDelta(d)
		}
	}
	if err != nil {
		return thread.AgentResult{}, err
	}
	return result, nil
}

const (
	testSessionID  = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	otherSessionID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
)

// fakeEventPublisher — thread.EventPublisher с записью всех вызовов
// (Stage 9) — mutex-guarded, вызывается из фоновой горутины
// (dispatchProcessing), читается из горутины теста.
type fakeEventPublisher struct {
	mu     sync.Mutex
	events []publishedEvent
}

type publishedEvent struct {
	kind     string // "status" | "answer_delta" | "answer_done" | "error"
	threadID string
	status   domain.ThreadStatus
	delta    string
	code     string
}

func newFakeEventPublisher() *fakeEventPublisher {
	return &fakeEventPublisher{}
}

func (p *fakeEventPublisher) record(ev publishedEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
}

func (p *fakeEventPublisher) all() []publishedEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]publishedEvent, len(p.events))
	copy(out, p.events)
	return out
}

func (p *fakeEventPublisher) PublishStatus(_ context.Context, threadID string, status domain.ThreadStatus, _ string) {
	p.record(publishedEvent{kind: "status", threadID: threadID, status: status})
}

func (p *fakeEventPublisher) PublishAnswerDelta(_ context.Context, threadID string, delta string) {
	p.record(publishedEvent{kind: "answer_delta", threadID: threadID, delta: delta})
}

func (p *fakeEventPublisher) PublishAnswerDone(_ context.Context, threadID string, _ domain.Message, status domain.ThreadStatus) {
	p.record(publishedEvent{kind: "answer_done", threadID: threadID, status: status})
}

func (p *fakeEventPublisher) PublishError(_ context.Context, threadID string, code, _ string) {
	p.record(publishedEvent{kind: "error", threadID: threadID, code: code})
}

type testSetup struct {
	repo    *fakeRepo
	balance *fakeBalance
	catalog *fakeCatalog
	files   *fakeFileAttacher
	agent   *fakeAgent
	events  *fakeEventPublisher
	svc     *thread.Service
	now     time.Time
}

func newTestSetup(agent *fakeAgent, ids []string) *testSetup {
	repo := newFakeRepo()
	bal := newFakeBalance()
	cat := newFakeCatalog()
	files := newFakeFileAttacher()
	events := newFakeEventPublisher()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := thread.New(repo, bal, cat, files, agent, events, clock.Fake{T: now}, &idgen.Fake{IDs: ids})
	return &testSetup{repo: repo, balance: bal, catalog: cat, files: files, agent: agent, events: events, svc: svc, now: now}
}

// waitForTerminal — Stage 9: CreateThread/AddMessage/Resume
// возвращаются сразу после перехода в Processing, реальное завершение
// раунда (Done/Error) происходит в фоновой горутине
// (dispatchProcessing). fakeAgent не делает реального I/O — раунд с ним
// завершается почти мгновенно, короткий poll-интервал здесь не признак
// хрупкости теста, а просто ожидание планировщика горутин.
func waitForTerminal(t *testing.T, repo *fakeRepo, threadID string) domain.Thread {
	t.Helper()
	var final domain.Thread
	require.Eventually(t, func() bool {
		th, err := repo.GetByID(context.Background(), threadID)
		if err != nil {
			return false
		}
		final = th
		return th.Status != domain.ThreadStatusProcessing
	}, 2*time.Second, time.Millisecond, "thread never left Processing")
	return final
}

// TestCreateThread_SufficientBalance_ReturnsProcessingThenFinishesToDoneInBackground —
// Stage 9: обработка фоновая (dispatchProcessing) — CreateThread возвращается
// сразу после перехода AwaitingPayment -> Processing, без ответа ассистента; финальный
// переход в Done и само сообщение появляются асинхронно (проверяется через
// repo, как их увидел бы WS-подписчик/повторный GET /threads/{id}).
func TestCreateThread_SufficientBalance_ReturnsProcessingThenFinishesToDoneInBackground(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "Ваш ответ готов."})
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
	require.Equal(t, domain.ThreadStatusProcessing, got.Status, "возвращается сразу после перехода в Processing, не дожидаясь агента")
	require.Equal(t, "Как оформить развод?", got.Title)
	require.Equal(t, 1, got.MessageCount, "ответ ассистента ещё не сгенерирован")

	final := waitForTerminal(t, setup.repo, got.ID)
	require.Equal(t, domain.ThreadStatusDone, final.Status)
	require.Equal(t, "Ваш ответ готов.", final.PreviewText)
	require.Equal(t, 2, final.MessageCount) // пользователь + ассистент
	require.Equal(t, 1, setup.agent.requestCount())

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
	svc := thread.New(repo, bal, newFakeCatalog(), newFakeFileAttacher(), agent, newFakeEventPublisher(), clk, &idgen.Fake{IDs: []string{"thread-1", "msg-user-1", "msg-assistant-1"}})

	got, err := svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, Language: domain.LanguageRu, ServiceID: "qa", Text: "Вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	waitForTerminal(t, repo, got.ID)

	msgs, err := repo.GetMessages(context.Background(), got.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	require.Nil(t, msgs[0].ProcessingTimeMs, "user message has no processing time")
	require.NotNil(t, msgs[1].ProcessingTimeMs, "assistant message must carry processing_time_ms (Stage 7 analytics)")
	require.Positive(t, *msgs[1].ProcessingTimeMs)
}

func TestCreateThread_InsufficientBalance_CreatesAwaitingPayment(t *testing.T) {
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
	require.Equal(t, domain.ThreadStatusAwaitingPayment, got.Status)
	require.Equal(t, 0, setup.agent.requestCount(), "агент не должен вызываться для неоплаченного треда")
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
	final := waitForTerminal(t, setup.repo, got.ID)
	require.Equal(t, domain.ThreadStatusDone, final.Status)
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
	final := waitForTerminal(t, setup.repo, got.ID)
	require.Equal(t, domain.ThreadStatusError, final.Status)

	// finishWithError возвращает кредит ПОСЛЕ перехода статуса в Error
	// (см. thread.go) — waitForTerminal ловит момент смены статуса, но
	// RefundCredit может доехать на пару инструкций позже той же горутины,
	// отдельный короткий Eventually на сам факт возврата.
	require.Eventually(t, func() bool {
		return len(setup.balance.refundsSnapshot()) == 1
	}, 2*time.Second, time.Millisecond)
	refunds := setup.balance.refundsSnapshot()
	require.Equal(t, refundCall{testSessionID, "qa", 1}, refunds[0])
	balance, _ := setup.balance.GetBalance(context.Background(), testSessionID)
	require.Contains(t, balance, domain.UserCredit{ServiceID: "qa", Quantity: 1}) // списали и тут же вернули
}

// TestAddMessage_OnDoneThread_IsNewPaidConsultation — один вопрос = одна
// консультация: новый вопрос в отвеченном треде списывает ещё одну единицу.
func TestAddMessage_OnDoneThread_IsNewPaidConsultation(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "первый ответ"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1", "msg-user-2", "msg-assistant-2"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 2

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос 1", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	created = waitForTerminal(t, setup.repo, created.ID)
	require.Equal(t, domain.ThreadStatusDone, created.Status)

	agent.setResult(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "второй ответ"})
	updated, err := setup.svc.AddMessage(context.Background(), thread.AddMessageRequest{
		ThreadID: created.ID, SessionID: testSessionID, Text: "вопрос 2", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusProcessing, updated.Status, "AddMessage тоже возвращается сразу, не дожидаясь агента")

	final := waitForTerminal(t, setup.repo, updated.ID)
	require.Equal(t, domain.ThreadStatusDone, final.Status)
	require.Equal(t, 4, final.MessageCount)
	require.Equal(t, "второй ответ", final.PreviewText)

	balance, _ := setup.balance.GetBalance(context.Background(), testSessionID)
	require.Contains(t, balance, domain.UserCredit{ServiceID: "qa", Quantity: 0}, "за каждый вопрос — одна единица")
}

func TestAddMessage_OnDoneThreadWithoutBalance_AwaitsPayment(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "первый ответ"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1", "msg-user-2"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос 1", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	created = waitForTerminal(t, setup.repo, created.ID)

	got, err := setup.svc.AddMessage(context.Background(), thread.AddMessageRequest{
		ThreadID: created.ID, SessionID: testSessionID, Text: "вопрос 2", InputType: domain.MessageInputTypeText,
	})

	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusAwaitingPayment, got.Status)
	require.Equal(t, 3, got.MessageCount, "вопрос сохранён, ждёт оплаты")
	require.Equal(t, 1, agent.requestCount())
}

// TestAddMessage_WithFileIDs_AttachesToNewMessage — новый вопрос с вложением
// (input_type=file, без текста) внутри уже открытого треда (Stage 4).
func TestAddMessage_WithFileIDs_AttachesToNewMessage(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "первый ответ"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1", "msg-user-2", "msg-assistant-2"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 2

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос 1", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	created = waitForTerminal(t, setup.repo, created.ID) // иначе AddMessage ниже попадёт на ErrThreadBusy (тред ещё Processing)

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

// createAwaitingThread — неоплаченный тред: баланса на момент создания нет.
func createAwaitingThread(t *testing.T, setup *testSetup) domain.Thread {
	t.Helper()
	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, Language: domain.LanguageRu, ServiceID: "qa", Text: "вопрос без баланса", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusAwaitingPayment, created.Status)
	return created
}

func TestAddMessage_OnAwaitingPaymentWithoutBalance_SavesMessageAndStaysAwaiting(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-user-2"})
	created := createAwaitingThread(t, setup)

	got, err := setup.svc.AddMessage(context.Background(), thread.AddMessageRequest{
		ThreadID: created.ID, SessionID: testSessionID, Text: "новый вопрос", InputType: domain.MessageInputTypeText,
	})

	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusAwaitingPayment, got.Status)
	require.Equal(t, 2, got.MessageCount, "новый вопрос сохраняется и без оплаты")
	require.Equal(t, 0, agent.requestCount())
}

func TestAddMessage_OnAwaitingPaymentWithBalance_DebitsAndProcesses(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "ответ"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-user-2", "msg-assistant-1"})
	created := createAwaitingThread(t, setup)
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1 // пополнил баланс в «Тарифах»

	got, err := setup.svc.AddMessage(context.Background(), thread.AddMessageRequest{
		ThreadID: created.ID, SessionID: testSessionID, Text: "новый вопрос", InputType: domain.MessageInputTypeText,
	})

	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusProcessing, got.Status)
	require.Equal(t, domain.ThreadStatusDone, waitForTerminal(t, setup.repo, got.ID).Status)
	balance, _ := setup.balance.GetBalance(context.Background(), testSessionID)
	require.Contains(t, balance, domain.UserCredit{ServiceID: "qa", Quantity: 0})
}

func TestAddMessage_OnErrorThread_ReturnsNotActive(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultError})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	created = waitForTerminal(t, setup.repo, created.ID)
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

func TestResume_WithBalance_DebitsAndProcessesSavedQuestion(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "готово после оплаты"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1"})
	created := createAwaitingThread(t, setup)

	// Баланс появился (пополнение в «Тарифах» или confirm{thread_id}).
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	activated, err := setup.svc.Resume(context.Background(), created.ID, testSessionID, domain.LanguageRu)
	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusProcessing, activated.Status, "возвращается сразу, не дожидаясь агента")

	final := waitForTerminal(t, setup.repo, activated.ID)
	require.Equal(t, domain.ThreadStatusDone, final.Status)
	require.Equal(t, 2, final.MessageCount, "ответ на уже сохранённый вопрос, без нового сообщения")
	balance, _ := setup.balance.GetBalance(context.Background(), testSessionID)
	require.Contains(t, balance, domain.UserCredit{ServiceID: "qa", Quantity: 0}, "единица списана с баланса")
}

func TestResume_WithoutBalance_StaysAwaitingPayment(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1"})
	created := createAwaitingThread(t, setup)

	got, err := setup.svc.Resume(context.Background(), created.ID, testSessionID, domain.LanguageRu)

	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusAwaitingPayment, got.Status)
	require.Equal(t, 0, agent.requestCount())
}

func TestResume_LostActivationRace_RefundsDebitedCredit(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1"})
	created := createAwaitingThread(t, setup)
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1
	setup.repo.activateLosesRace = true

	_, err := setup.svc.Resume(context.Background(), created.ID, testSessionID, domain.LanguageRu)

	require.NoError(t, err)
	balance, _ := setup.balance.GetBalance(context.Background(), testSessionID)
	require.Contains(t, balance, domain.UserCredit{ServiceID: "qa", Quantity: 1}, "списанная единица вернулась")
	require.Equal(t, 0, agent.requestCount())
}

func TestResume_NotAwaitingPayment_IsNoOp(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "готово"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusProcessing, created.Status)
	created = waitForTerminal(t, setup.repo, created.ID)
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1 // например, доплата за другой тред

	again, err := setup.svc.Resume(context.Background(), created.ID, testSessionID, domain.LanguageRu)
	require.NoError(t, err)
	require.Equal(t, created.Status, again.Status)
	require.Equal(t, 1, agent.requestCount(), "повторная активация не должна снова вызывать агента")
	balance, _ := setup.balance.GetBalance(context.Background(), testSessionID)
	require.Contains(t, balance, domain.UserCredit{ServiceID: "qa", Quantity: 1}, "no-op не списывает баланс")
}

func TestCancelThread_FromAwaitingPayment_Succeeds(t *testing.T) {
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
	created = waitForTerminal(t, setup.repo, created.ID)
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
	waitForTerminal(t, setup.repo, created.ID)

	_, msgs, err := setup.svc.GetThread(context.Background(), created.ID, testSessionID)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assistantMsgID := msgs[1].ID

	updated, err := setup.svc.SetMessageFeedback(context.Background(), assistantMsgID, testSessionID, domain.MessageFeedbackLike)
	require.NoError(t, err)
	require.NotNil(t, updated.Feedback)
	require.Equal(t, domain.MessageFeedbackLike, *updated.Feedback)
}

// --- Stage 9: фоновая обработка + WS-события (dispatchProcessing,
// EventPublisher) ---

// TestCreateThread_ReturnsBeforeAgentFinishes — ключевое поведение Stage 9:
// CreateThread не блокируется на Agent.Process. fakeAgent.block держит
// Process заблокированным, пока тест явно не отпустит его — если бы
// CreateThread всё ещё был синхронным, сам вызов CreateThread завис бы
// на этом канале и тест бы не дошёл до строки ниже за отведённый таймаут.
func TestCreateThread_ReturnsBeforeAgentFinishes(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "готово"})
	agent.block = make(chan struct{})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	done := make(chan domain.Thread, 1)
	go func() {
		got, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
			SessionID: testSessionID, ServiceID: "qa", Text: "вопрос", InputType: domain.MessageInputTypeText,
		})
		require.NoError(t, err)
		done <- got
	}()

	var got domain.Thread
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CreateThread не вернулся — похоже, он всё ещё ждёт Agent.Process синхронно")
	}
	require.Equal(t, domain.ThreadStatusProcessing, got.Status)
	require.Equal(t, 1, got.MessageCount, "сообщение ассистента ещё не создано")

	close(agent.block) // отпускаем Process — раунд теперь может завершиться
	final := waitForTerminal(t, setup.repo, got.ID)
	require.Equal(t, domain.ThreadStatusDone, final.Status)
}

// TestAddMessage_PublishesStatusThenDeltasThenDone_InOrder — WS-события
// (internal/wshub.Hub в проде, fakeEventPublisher здесь) публикуются в
// строгом порядке: processing -> дельты Фазы 1 (в порядке генерации) ->
// answer_done. Порядок важен клиенту (WS-подписчик рендерит стрим по мере
// прихода) — тест фиксирует его как контракт thread.Service, а не деталь
// реализации.
func TestAddMessage_PublishesStatusThenDeltasThenDone_InOrder(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "первый ответ"})
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1", "msg-assistant-1", "msg-user-2", "msg-assistant-2"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 2 // два вопроса — две консультации

	created, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос 1", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	created = waitForTerminal(t, setup.repo, created.ID)
	require.Equal(t, domain.ThreadStatusDone, created.Status)

	agent.deltas = []string{"Прив", "ет, ", "мир"}
	agent.setResult(thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "Привет, мир"})
	_, err = setup.svc.AddMessage(context.Background(), thread.AddMessageRequest{
		ThreadID: created.ID, SessionID: testSessionID, Text: "вопрос 2", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err)
	waitForTerminal(t, setup.repo, created.ID)

	// Оставляем только события ВТОРОГО раунда (после первого Done) — ищем
	// первое answer_done и берём всё после него.
	all := setup.events.all()
	firstDoneIdx := -1
	for i, ev := range all {
		if ev.kind == "answer_done" {
			firstDoneIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, firstDoneIdx, 0)
	roundTwo := all[firstDoneIdx+1:]

	require.NotEmpty(t, roundTwo)
	require.Equal(t, "status", roundTwo[0].kind)
	require.Equal(t, domain.ThreadStatusProcessing, roundTwo[0].status)

	var deltas []string
	for _, ev := range roundTwo[1:] {
		if ev.kind == "answer_delta" {
			deltas = append(deltas, ev.delta)
		}
	}
	require.Equal(t, []string{"Прив", "ет, ", "мир"}, deltas)

	last := roundTwo[len(roundTwo)-1]
	require.Equal(t, "answer_done", last.kind)
	require.Equal(t, domain.ThreadStatusDone, last.status)
}

// TestRunAgentAndFinish_PanicInAgentIsRecoveredAndMarksThreadError —
// паника внутри Agent.Process (фоновая горутина, dispatchProcessing) не
// должна уронить процесс и не должна оставить тред зависшим в Processing
// навсегда — recover переводит его в Error тем же путём, что обычный сбой
// агента, включая возврат кредита.
func TestRunAgentAndFinish_PanicInAgentIsRecoveredAndMarksThreadError(t *testing.T) {
	agent := newFakeAgent(thread.AgentResult{})
	agent.panicWith = "boom: simulated agent panic"
	setup := newTestSetup(agent, []string{"thread-1", "msg-user-1"})
	setup.balance.credits[balanceKey(testSessionID, "qa")] = 1

	got, err := setup.svc.CreateThread(context.Background(), thread.CreateThreadRequest{
		SessionID: testSessionID, ServiceID: "qa", Text: "вопрос", InputType: domain.MessageInputTypeText,
	})
	require.NoError(t, err, "CreateThread сам по себе не должен видеть панику — она внутри фоновой горутины")

	final := waitForTerminal(t, setup.repo, got.ID)
	require.Equal(t, domain.ThreadStatusError, final.Status, "паника должна деградировать до обычного error-пути, не оставлять Processing")

	require.Eventually(t, func() bool {
		return len(setup.balance.refundsSnapshot()) == 1
	}, 2*time.Second, time.Millisecond, "кредит должен вернуться даже при панике агента")
}
