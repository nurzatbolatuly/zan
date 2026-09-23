package httpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"zan-backend/internal/httpserver"
	"zan-backend/internal/platform/bruteforce"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/platform/ratelimit"
	"zan-backend/internal/service/billing"
	"zan-backend/internal/service/catalog"
	"zan-backend/internal/service/session"
	"zan-backend/internal/service/thread"
	"zan-backend/internal/wshub"
)

// gatedAgent — thread.Agent (Stage 9) с явным контролем темпа эмиссии
// дельт: Process ждёт сигнала в advance перед КАЖДОЙ дельтой (включая
// финальный "сигнал" перед возвратом результата) и подтверждает через ack,
// что onDelta для этой дельты уже вызван (значит PublishAnswerDelta уже
// долетел до hub) — тест может детерминированно застать состояние "N из M
// дельт уже опубликованы, никто не подключился" без sleep/поллинга. Нужен
// только тестам позднего/раннего коннекта ниже — остальные WS-тесты
// используют обычный router_test.go#fakeAgent (мгновенный, без пауз).
type gatedAgent struct {
	deltas  []string
	result  thread.AgentResult
	advance chan struct{}
	ack     chan struct{}
}

func (a *gatedAgent) Process(_ context.Context, req thread.AgentRequest) (thread.AgentResult, error) {
	for _, d := range a.deltas {
		<-a.advance
		if req.OnDelta != nil {
			req.OnDelta(d)
		}
		a.ack <- struct{}{}
	}
	<-a.advance
	return a.result, nil
}

// newWSTestDeps — как router_test.go#newTestDeps, но с внедряемым agent
// (нужен gatedAgent для тестов раннего/позднего коннекта) —
// Documents/Analytics/Files/Voice/Prompts намеренно nil, WS-тесты их не
// касаются.
func newWSTestDeps(agent thread.Agent) httpserver.Deps {
	sessions := session.New(newFakeSessionRepo(), clock.Real{}, idgen.UUIDGenerator{}, session.NewTokenSigner("test-secret"))
	cat := catalog.New(newFakeCatalogRepo(), idgen.UUIDGenerator{})
	bill := billing.New(newFakeBillingRepo(), cat, clock.Real{}, idgen.UUIDGenerator{})
	threadRepo := newFakeThreadRepo()
	hub := wshub.NewHub()
	th := thread.New(threadRepo, bill, cat, noopFileAttacher{}, agent, hub, clock.Real{}, idgen.UUIDGenerator{})
	return httpserver.Deps{
		Sessions:               sessions,
		Catalog:                cat,
		Billing:                bill,
		Thread:                 th,
		Hub:                    hub,
		AdminToken:             testAdminToken,
		SessionRateLimit:       ratelimit.NewStore(1000, 1000),
		ThreadRateLimit:        ratelimit.NewStore(1000, 1000),
		ThreadSessionRateLimit: ratelimit.NewStore(1000, 1000),
		ClientLogRateLimit:     ratelimit.NewStore(1000, 1000),
		AdminBruteforce:        bruteforce.NewStore(1000, time.Minute, clock.Real{}),
	}
}

func newWSTestServer(t *testing.T, deps httpserver.Deps) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	l := logger.New(io.Discard, slog.LevelDebug)
	srv := httptest.NewServer(httpserver.NewRouter(l, deps))
	t.Cleanup(srv.Close)
	return srv
}

// wsThreadEventBody — зеркало wire-формы событий GET /ws/threads/{id}
// (internal/httpserver/ws_thread.go#wsThreadEvent) для тестов — сверяем по
// JSON с внешней стороны (пакет httpserver_test), не по внутреннему типу.
type wsThreadEventBody struct {
	Type        string `json:"type"`
	ThreadID    string `json:"thread_id"`
	Seq         int    `json:"seq"`
	Status      string `json:"status,omitempty"`
	PreviewText string `json:"preview_text,omitempty"`
	Delta       string `json:"delta,omitempty"`
	Message     *struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"message,omitempty"`
	Code         string `json:"code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

func readWSEvent(t *testing.T, ctx context.Context, conn *websocket.Conn) wsThreadEventBody {
	t.Helper()
	_, data, err := conn.Read(ctx)
	require.NoError(t, err)
	var ev wsThreadEventBody
	require.NoError(t, json.Unmarshal(data, &ev))
	return ev
}

func wsURL(httpURL, path string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http") + path
}

// createThreadAsync — POST /threads блокируется, пока фоновый раунд не
// начнёт ждать на gatedAgent.advance (сам HTTP-хендлер возвращается сразу
// после перехода в Processing — Stage 9 — так что на деле этот вызов
// возвращается быстро; горутина здесь просто даёт тесту структуру
// "запустили — получили id — дальше управляем темпом через advance/ack"
// без блокировки основного потока теста на самом запросе).
func createThreadAsync(t *testing.T, router http.Handler, token string, payload map[string]any) threadDetailBody {
	t.Helper()
	rec := createThread(t, router, token, payload)
	require.Equal(t, http.StatusCreated, rec.Code)
	var body threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func TestThreadWebSocket_FullRoundTrip(t *testing.T) {
	agent := &gatedAgent{
		deltas:  []string{"Здрав", "ствуйте"},
		result:  thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "Здравствуйте"},
		advance: make(chan struct{}),
		ack:     make(chan struct{}),
	}
	deps := newWSTestDeps(agent)
	srv := newWSTestServer(t, deps)
	router := srv.Config.Handler

	_, token, _ := createTestSession(t, router)
	paymentID, _, status := checkout(t, router, token, map[string]any{
		"kind": "custom", "items": []map[string]any{{"service_id": "qa", "qty": 1}},
	})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)

	// createThread сама по себе не паникует/не зовёт require (см. её
	// определение в thread_test.go) — безопасна для вызова из отдельной
	// горутины; require.* на теле ответа делаем здесь, в основной горутине
	// теста, как того требует testing.T (t.FailNow — только из горутины
	// самого теста).
	recCh := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recCh <- createThread(t, router, token, map[string]any{"service_id": "qa", "text": "Привет", "input_type": "text"})
	}()

	var rec *httptest.ResponseRecorder
	select {
	case rec = <-recCh:
	case <-time.After(2 * time.Second):
		t.Fatal("createThread didn't return")
	}
	require.Equal(t, http.StatusCreated, rec.Code)
	var createdBody threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createdBody))
	threadID := createdBody.ID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, wsURL(srv.URL, "/ws/threads/"+threadID+"?token="+token), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	defer func() { _ = conn.CloseNow() }()

	first := readWSEvent(t, ctx, conn)
	require.Equal(t, "thread_status", first.Type)
	require.Equal(t, "processing", first.Status)

	agent.advance <- struct{}{}
	<-agent.ack
	d1 := readWSEvent(t, ctx, conn)
	require.Equal(t, "answer_delta", d1.Type)
	require.Equal(t, "Здрав", d1.Delta)

	agent.advance <- struct{}{}
	<-agent.ack
	d2 := readWSEvent(t, ctx, conn)
	require.Equal(t, "answer_delta", d2.Type)
	require.Equal(t, "ствуйте", d2.Delta)

	agent.advance <- struct{}{} // финальный релиз — раунд завершается, Process возвращает result

	done := readWSEvent(t, ctx, conn)
	require.Equal(t, "answer_done", done.Type)
	require.Equal(t, "done", done.Status)
	require.NotNil(t, done.Message)
	require.Equal(t, "Здравствуйте", done.Message.Text)

	final := waitForThreadStatus(t, router, token, threadID)
	require.Equal(t, "done", final.Status)
}

func TestThreadWebSocket_InvalidToken_RejectsBeforeUpgrade(t *testing.T) {
	deps := newWSTestDeps(fakeAgent{})
	srv := newWSTestServer(t, deps)
	router := srv.Config.Handler
	_, token, _ := createTestSession(t, router)
	paymentID, _, status := checkout(t, router, token, map[string]any{
		"kind": "custom", "items": []map[string]any{{"service_id": "qa", "qty": 1}},
	})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)
	created := createThreadAsync(t, router, token, map[string]any{"service_id": "qa", "text": "вопрос", "input_type": "text"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, wsURL(srv.URL, "/ws/threads/"+created.ID+"?token=not-a-real-token"), nil)
	require.Error(t, err, "upgrade must be rejected for an invalid session token")
	if resp != nil {
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
}

func TestThreadWebSocket_ThreadOwnedByOtherSession_RejectsBeforeUpgrade(t *testing.T) {
	deps := newWSTestDeps(fakeAgent{})
	srv := newWSTestServer(t, deps)
	router := srv.Config.Handler
	_, token, _ := createTestSession(t, router)
	paymentID, _, status := checkout(t, router, token, map[string]any{
		"kind": "custom", "items": []map[string]any{{"service_id": "qa", "qty": 1}},
	})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)
	created := createThreadAsync(t, router, token, map[string]any{"service_id": "qa", "text": "вопрос", "input_type": "text"})

	_, otherToken, _ := createTestSession(t, router)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, wsURL(srv.URL, "/ws/threads/"+created.ID+"?token="+otherToken), nil)
	require.Error(t, err)
	if resp != nil {
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	}
}

// TestThreadWebSocket_LateConnect_SnapshotIncludesAlreadyEmittedDeltas —
// Stage 9, ключевой инвариант хаба (internal/wshub): клиент, подключившийся
// ПОСЛЕ того, как часть дельт уже улетела без единого подписчика, первым
// же кадром получает снапшот с уже накопленным текстом (одной "большой"
// дельтой), а не только последующие живые события — и ничего не теряет.
func TestThreadWebSocket_LateConnect_SnapshotIncludesAlreadyEmittedDeltas(t *testing.T) {
	agent := &gatedAgent{
		deltas:  []string{"раз", "два", "три"},
		result:  thread.AgentResult{Status: thread.AgentResultDone, AnswerText: "раздватри"},
		advance: make(chan struct{}),
		ack:     make(chan struct{}),
	}
	deps := newWSTestDeps(agent)
	srv := newWSTestServer(t, deps)
	router := srv.Config.Handler
	_, token, _ := createTestSession(t, router)
	paymentID, _, status := checkout(t, router, token, map[string]any{
		"kind": "custom", "items": []map[string]any{{"service_id": "qa", "qty": 1}},
	})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)

	recCh := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recCh <- createThread(t, router, token, map[string]any{"service_id": "qa", "text": "вопрос", "input_type": "text"})
	}()
	var rec *httptest.ResponseRecorder
	select {
	case rec = <-recCh:
	case <-time.After(2 * time.Second):
		t.Fatal("createThread didn't return")
	}
	require.Equal(t, http.StatusCreated, rec.Code)
	var createdBody threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createdBody))
	threadID := createdBody.ID

	// Отпускаем ровно 2 из 3 дельт БЕЗ единого подписчика — ack гарантирует,
	// что PublishAnswerDelta уже долетел до hub к моменту, когда мы
	// подключаемся ниже; publish не блокируется отсутствием слушателя.
	agent.advance <- struct{}{}
	<-agent.ack
	agent.advance <- struct{}{}
	<-agent.ack

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, wsURL(srv.URL, "/ws/threads/"+threadID+"?token="+token), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	defer func() { _ = conn.CloseNow() }()

	first := readWSEvent(t, ctx, conn)
	require.Equal(t, "thread_status", first.Type)
	require.Equal(t, "processing", first.Status)

	snapshot := readWSEvent(t, ctx, conn)
	require.Equal(t, "answer_delta", snapshot.Type)
	require.Equal(t, "раздва", snapshot.Delta, "снапшот — накопленный текст ОБЕИХ уже отправленных дельт одним событием")

	// Отпускаем оставшуюся дельту и финал — теперь уже с живым подписчиком.
	agent.advance <- struct{}{}
	<-agent.ack
	live := readWSEvent(t, ctx, conn)
	require.Equal(t, "answer_delta", live.Type)
	require.Equal(t, "три", live.Delta)

	agent.advance <- struct{}{}
	done := readWSEvent(t, ctx, conn)
	require.Equal(t, "answer_done", done.Type)
}
