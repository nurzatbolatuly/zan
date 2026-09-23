package httpserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func createThread(t *testing.T, router http.Handler, token string, payload map[string]any) (rec *httptest.ResponseRecorder) {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/threads", bytes.NewReader(body))
	req.Header.Set("Authorization", "Session "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// waitForThreadStatus — Stage 9: обработка фоновая (thread.Service#
// dispatchProcessing), POST /threads и POST /threads/{id}/messages
// возвращаются сразу после перехода в Processing. Тесты, проверяющие
// эффект завершённого раунда (ответ ассистента, финальный статус),
// поллят GET /threads/{id} до тех пор, пока статус не перестанет быть
// "processing" — fakeAgent (router_test.go) не делает реального I/O,
// раунд завершается почти мгновенно.
func waitForThreadStatus(t *testing.T, router http.Handler, token, threadID string) threadDetailBody {
	t.Helper()
	var last threadDetailBody
	require.Eventually(t, func() bool {
		req := httptest.NewRequest(http.MethodGet, "/threads/"+threadID, nil)
		req.Header.Set("Authorization", "Session "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			return false
		}
		var body threadDetailBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			return false
		}
		last = body
		return body.Status != "processing"
	}, 2*time.Second, time.Millisecond, "thread never left processing")
	return last
}

type threadDetailBody struct {
	ID           string `json:"id"`
	ServiceID    string `json:"service_id"`
	Status       string `json:"status"`
	Title        string `json:"title"`
	PreviewText  string `json:"preview_text"`
	MessageCount int    `json:"message_count"`
	Messages     []struct {
		ID     string `json:"id"`
		Sender string `json:"sender"`
		Text   string `json:"text"`
	} `json:"messages"`
}

func TestCreateThread_WithoutSession_Returns401(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := createThread(t, router, "", map[string]any{"service_id": "qa", "text": "Вопрос", "input_type": "text"})

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestCreateThread_SufficientBalance_ReturnsProcessingThenFinishesToDone —
// Stage 9: ответ POST /threads отражает тред сразу после перехода в
// Processing (ответа ассистента там ещё нет) — сам ответ приходит
// асинхронно, здесь проверяется через GET /threads/{id} (WS-подписчик в
// проде увидел бы то же самое через GET /ws/threads/{id}).
func TestCreateThread_SufficientBalance_ReturnsProcessingThenFinishesToDone(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	paymentID, _, status := checkout(t, router, token, map[string]any{
		"kind":  "custom",
		"items": []map[string]any{{"service_id": "qa", "qty": 1}},
	})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)

	rec := createThread(t, router, token, map[string]any{
		"service_id": "qa",
		"text":       "Как оформить развод?",
		"input_type": "text",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var body threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "processing", body.Status)
	require.Equal(t, "Как оформить развод?", body.Title)
	require.Len(t, body.Messages, 1, "ответ ассистента ещё не сгенерирован")

	final := waitForThreadStatus(t, router, token, body.ID)
	require.Equal(t, "done", final.Status)
	require.Len(t, final.Messages, 2)
	require.Equal(t, "assistant", final.Messages[1].Sender)
	require.NotEmpty(t, final.Messages[1].Text)
}

func TestCreateThread_InsufficientBalance_AwaitingPaymentThenActivatedByCheckoutConfirm(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	rec := createThread(t, router, token, map[string]any{
		"service_id": "qa",
		"text":       "Вопрос без баланса",
		"input_type": "text",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, "awaiting_payment", created.Status)

	paymentID, _, status := checkout(t, router, token, map[string]any{
		"kind":      "single_service",
		"items":     []map[string]any{{"service_id": "qa", "qty": 1}},
		"thread_id": created.ID,
	})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)

	activated := waitForThreadStatus(t, router, token, created.ID)
	require.Equal(t, "done", activated.Status)
	// confirm начислил единицу, активация треда её же и списала — не «тред
	// оплачен + лишний кредит на балансе».
	require.Contains(t, getBalance(t, router, token), `"service_id":"qa","quantity":0`)
}

// TestResumeThread_AfterTopUpInTariffs_ProcessesSavedQuestion — отказался
// от оплаты при отправке (тред остался awaiting_payment), пополнил баланс
// через «Свой набор» (платёж без thread_id), вернулся и перезапустил вопрос.
func TestResumeThread_AfterTopUpInTariffs_ProcessesSavedQuestion(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	rec := createThread(t, router, token, map[string]any{"service_id": "qa", "text": "вопрос", "input_type": "text"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	stillAwaiting := resumeThread(t, router, token, created.ID)
	require.Equal(t, "awaiting_payment", stillAwaiting.Status, "без баланса перезапуск ничего не списывает")

	paymentID, _, status := checkout(t, router, token, map[string]any{
		"kind":  "custom",
		"items": []map[string]any{{"service_id": "qa", "qty": 1}},
	})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)
	require.Equal(t, "awaiting_payment", waitForThreadStatus(t, router, token, created.ID).Status,
		"пополнение без thread_id не запускает тред само")

	resumed := resumeThread(t, router, token, created.ID)
	require.Equal(t, "processing", resumed.Status)

	final := waitForThreadStatus(t, router, token, created.ID)
	require.Equal(t, "done", final.Status)
	require.Len(t, final.Messages, 2, "ответ на сохранённый вопрос")
	require.Contains(t, getBalance(t, router, token), `"service_id":"qa","quantity":0`)
}

func resumeThread(t *testing.T, router http.Handler, token, threadID string) threadDetailBody {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/threads/"+threadID+"/resume", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var body threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func getBalance(t *testing.T, router http.Handler, token string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/balance", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	return rec.Body.String()
}

// TestCreateThread_NullableFieldsSerializedAsNull — nullable-поля
// присутствуют в JSON явным null, не пропадают (openapi.yaml#Thread,
// #Message): фронт отличает "нет значения" сравнением с null, а не с
// undefined.
func TestCreateThread_NullableFieldsSerializedAsNull(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	rec := createThread(t, router, token, map[string]any{
		"service_id": "qa",
		"text":       "Вопрос без баланса",
		"input_type": "text",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var thread map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &thread))
	var messages []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(thread["messages"], &messages))
	require.NotEmpty(t, messages)
	for _, field := range []string{"feedback", "processing_time_ms"} {
		require.Contains(t, messages[0], field)
		require.JSONEq(t, "null", string(messages[0][field]), field)
	}
}

func confirmPayment(t *testing.T, router http.Handler, token, paymentID string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/payments/"+paymentID+"/confirm", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestGetThread_ForeignSession_Returns404(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	rec := createThread(t, router, token, map[string]any{"service_id": "qa", "text": "вопрос", "input_type": "text"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	_, otherToken, _ := createTestSession(t, router)
	req := httptest.NewRequest(http.MethodGet, "/threads/"+created.ID, nil)
	req.Header.Set("Authorization", "Session "+otherToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, req)

	require.Equal(t, http.StatusNotFound, getRec.Code)
	require.Contains(t, getRec.Body.String(), "thread_not_found")
}

func TestListThreads_ReturnsOnlyOwnThreads(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	_, otherToken, _ := createTestSession(t, router)

	require.Equal(t, http.StatusCreated, createThread(t, router, token, map[string]any{"service_id": "qa", "text": "мой вопрос", "input_type": "text"}).Code)
	require.Equal(t, http.StatusCreated, createThread(t, router, otherToken, map[string]any{"service_id": "qa", "text": "чужой вопрос", "input_type": "text"}).Code)

	req := httptest.NewRequest(http.MethodGet, "/threads", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Items []threadDetailBody `json:"items"`
		Total int                `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Total)
	require.Equal(t, "мой вопрос", resp.Items[0].Title)
}

// TestAddMessage_NewQuestionAfterDone_DebitsAnotherConsultation — один
// вопрос = одна консультация: второй вопрос в треде списывает вторую единицу.
func TestAddMessage_NewQuestionAfterDone_DebitsAnotherConsultation(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	paymentID, _, status := checkout(t, router, token, map[string]any{"kind": "custom", "items": []map[string]any{{"service_id": "qa", "qty": 2}}})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)

	rec := createThread(t, router, token, map[string]any{"service_id": "qa", "text": "первый вопрос", "input_type": "text"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	created = waitForThreadStatus(t, router, token, created.ID) // иначе POST .../messages ниже попадёт на 409 thread_busy
	require.Equal(t, "done", created.Status)

	body, err := json.Marshal(map[string]any{"text": "второй вопрос", "input_type": "text"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/threads/"+created.ID+"/messages", bytes.NewReader(body))
	req.Header.Set("Authorization", "Session "+token)
	req.Header.Set("Content-Type", "application/json")
	msgRec := httptest.NewRecorder()
	router.ServeHTTP(msgRec, req)

	require.Equal(t, http.StatusOK, msgRec.Code)
	var updated threadDetailBody
	require.NoError(t, json.Unmarshal(msgRec.Body.Bytes(), &updated))
	require.Equal(t, "processing", updated.Status, "тоже возвращается сразу, не дожидаясь агента")

	final := waitForThreadStatus(t, router, token, updated.ID)
	require.Equal(t, "done", final.Status)
	require.Equal(t, 4, final.MessageCount)
	require.Contains(t, getBalance(t, router, token), `"service_id":"qa","quantity":0`)
}

func TestCancelThread_AwaitingPayment_Succeeds(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	rec := createThread(t, router, token, map[string]any{"service_id": "qa", "text": "вопрос", "input_type": "text"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, "awaiting_payment", created.Status)

	req := httptest.NewRequest(http.MethodPost, "/threads/"+created.ID+"/cancel", nil)
	req.Header.Set("Authorization", "Session "+token)
	cancelRec := httptest.NewRecorder()
	router.ServeHTTP(cancelRec, req)

	require.Equal(t, http.StatusOK, cancelRec.Code)
	require.Contains(t, cancelRec.Body.String(), `"status":"canceled"`)
}

func TestDeleteThread_ThenGetReturns404(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	rec := createThread(t, router, token, map[string]any{"service_id": "qa", "text": "вопрос", "input_type": "text"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	delReq := httptest.NewRequest(http.MethodDelete, "/threads/"+created.ID, nil)
	delReq.Header.Set("Authorization", "Session "+token)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)
	require.Equal(t, http.StatusNoContent, delRec.Code)

	getReq := httptest.NewRequest(http.MethodGet, "/threads/"+created.ID, nil)
	getReq.Header.Set("Authorization", "Session "+token)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusNotFound, getRec.Code)
}

func TestMessageFeedback_SetsLike(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	paymentID, _, status := checkout(t, router, token, map[string]any{"kind": "custom", "items": []map[string]any{{"service_id": "qa", "qty": 1}}})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)

	rec := createThread(t, router, token, map[string]any{"service_id": "qa", "text": "вопрос", "input_type": "text"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	created = waitForThreadStatus(t, router, token, created.ID)
	assistantMessageID := created.Messages[1].ID

	body, err := json.Marshal(map[string]any{"value": "like"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/messages/"+assistantMessageID+"/feedback", bytes.NewReader(body))
	req.Header.Set("Authorization", "Session "+token)
	req.Header.Set("Content-Type", "application/json")
	fbRec := httptest.NewRecorder()
	router.ServeHTTP(fbRec, req)

	require.Equal(t, http.StatusOK, fbRec.Code)
	require.Contains(t, fbRec.Body.String(), `"feedback":"like"`)
}

func TestCreateThread_InvalidInputType_Returns400(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	rec := createThread(t, router, token, map[string]any{"service_id": "qa", "text": "вопрос", "input_type": "carrier_pigeon"})

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateThread_UnknownService_Returns404(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	rec := createThread(t, router, token, map[string]any{"service_id": "unknown", "text": "вопрос", "input_type": "text"})

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "service_not_found")
}
