package httpserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

type threadDetailBody struct {
	ID           string  `json:"id"`
	ServiceID    string  `json:"service_id"`
	Status       string  `json:"status"`
	Title        string  `json:"title"`
	PreviewText  string  `json:"preview_text"`
	MessageCount int     `json:"message_count"`
	IsPaid       bool    `json:"is_paid"`
	FreeUntil    *string `json:"free_until"`
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

func TestCreateThread_SufficientBalance_ProcessesSynchronouslyToDone(t *testing.T) {
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
	require.Equal(t, "done", body.Status)
	require.True(t, body.IsPaid)
	require.NotNil(t, body.FreeUntil)
	require.Equal(t, "Как оформить развод?", body.Title)
	require.Len(t, body.Messages, 2)
	require.Equal(t, "assistant", body.Messages[1].Sender)
	require.NotEmpty(t, body.Messages[1].Text)
}

func TestCreateThread_InsufficientBalance_QueuedThenActivatedByCheckoutConfirm(t *testing.T) {
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
	require.Equal(t, "queued", created.Status)
	require.False(t, created.IsPaid)

	paymentID, _, status := checkout(t, router, token, map[string]any{
		"kind":      "single_service",
		"items":     []map[string]any{{"service_id": "qa", "qty": 1}},
		"thread_id": created.ID,
	})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)

	req := httptest.NewRequest(http.MethodGet, "/threads/"+created.ID, nil)
	req.Header.Set("Authorization", "Session "+token)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, req)
	require.Equal(t, http.StatusOK, getRec.Code)

	var activated threadDetailBody
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &activated))
	require.Equal(t, "done", activated.Status)
	require.True(t, activated.IsPaid)
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

func TestAddMessage_ContinuesThreadAfterDone(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	paymentID, _, status := checkout(t, router, token, map[string]any{"kind": "custom", "items": []map[string]any{{"service_id": "qa", "qty": 1}}})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)

	rec := createThread(t, router, token, map[string]any{"service_id": "qa", "text": "первый вопрос", "input_type": "text"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, "done", created.Status)

	body, err := json.Marshal(map[string]any{"text": "уточняющий вопрос", "input_type": "text"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/threads/"+created.ID+"/messages", bytes.NewReader(body))
	req.Header.Set("Authorization", "Session "+token)
	req.Header.Set("Content-Type", "application/json")
	msgRec := httptest.NewRecorder()
	router.ServeHTTP(msgRec, req)

	require.Equal(t, http.StatusOK, msgRec.Code)
	var updated threadDetailBody
	require.NoError(t, json.Unmarshal(msgRec.Body.Bytes(), &updated))
	require.Equal(t, "done", updated.Status)
	require.Equal(t, 4, updated.MessageCount)
}

func TestCancelThread_QueuedUnpaid_Succeeds(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	rec := createThread(t, router, token, map[string]any{"service_id": "qa", "text": "вопрос", "input_type": "text"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, "queued", created.Status)

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
