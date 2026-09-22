package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type generateDocumentBody struct {
	ID         string `json:"id"`
	Sender     string `json:"sender"`
	Text       string `json:"text"`
	FilesReady bool   `json:"files_ready"`
	Findings   []struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	} `json:"findings"`
}

// createPaidThread — создаёт тред с достаточным балансом "qa" (fakeAgent
// синхронно отвечает Done, см. router_test.go), возвращает его id.
func createPaidThread(t *testing.T, router http.Handler, token string) string {
	t.Helper()
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
	var created threadDetailBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	return created.ID
}

func generateDocument(t *testing.T, router http.Handler, token, threadID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/threads/"+threadID+"/generate-document", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestGenerateDocument_WithoutSession_Returns401(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := generateDocument(t, router, "", "11111111-1111-1111-1111-111111111111")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestGenerateDocument_UnknownThread_Returns404(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	rec := generateDocument(t, router, token, "99999999-9999-9999-9999-999999999999")

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "thread_not_found")
}

func TestGenerateDocument_ForeignThread_Returns404(t *testing.T) {
	router, _ := newTestRouter(t)
	_, ownerToken, _ := createTestSession(t, router)
	threadID := createPaidThread(t, router, ownerToken)

	_, otherToken, _ := createTestSession(t, router)
	rec := generateDocument(t, router, otherToken, threadID)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGenerateDocument_InsufficientBalance_Returns409PaymentRequired(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	threadID := createPaidThread(t, router, token)

	// Баланс "doc" не пополнялся — только "qa" (createPaidThread).
	rec := generateDocument(t, router, token, threadID)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "payment_required")
}

func TestGenerateDocument_SufficientBalance_ReturnsMessageWithFilesReady(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	threadID := createPaidThread(t, router, token)

	paymentID, _, status := checkout(t, router, token, map[string]any{
		"kind":      "single_service",
		"items":     []map[string]any{{"service_id": "doc", "qty": 1}},
		"thread_id": threadID,
	})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)

	rec := generateDocument(t, router, token, threadID)

	require.Equal(t, http.StatusCreated, rec.Code)
	var body generateDocumentBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "assistant", body.Sender)
	require.NotEmpty(t, body.Text)
	require.True(t, body.FilesReady)
	require.Len(t, body.Findings, 1)

	// Сообщение реально попало в тред.
	getReq := httptest.NewRequest(http.MethodGet, "/threads/"+threadID, nil)
	getReq.Header.Set("Authorization", "Session "+token)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code)
	var detail threadDetailBody
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &detail))
	require.Len(t, detail.Messages, 3) // вопрос + qa-ответ + сгенерированный документ
}

func getThreadDocument(t *testing.T, router http.Handler, token, threadID, format string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/threads/"+threadID+"/document?format="+format, nil)
	req.Header.Set("Authorization", "Session "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestGetThreadDocument_NotGeneratedYet_Returns404(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	threadID := createPaidThread(t, router, token)

	rec := getThreadDocument(t, router, token, threadID, "pdf")

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "document_not_generated")
}

func TestGetThreadDocument_InvalidFormat_Returns400(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	threadID := createPaidThread(t, router, token)

	rec := getThreadDocument(t, router, token, threadID, "txt")

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetThreadDocument_AfterGenerate_ReturnsDownloadURL(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	threadID := createPaidThread(t, router, token)

	paymentID, _, status := checkout(t, router, token, map[string]any{
		"kind":      "single_service",
		"items":     []map[string]any{{"service_id": "doc", "qty": 1}},
		"thread_id": threadID,
	})
	require.Equal(t, http.StatusCreated, status)
	confirmPayment(t, router, token, paymentID)
	require.Equal(t, http.StatusCreated, generateDocument(t, router, token, threadID).Code)

	pdfRec := getThreadDocument(t, router, token, threadID, "pdf")
	require.Equal(t, http.StatusOK, pdfRec.Code)
	var pdfBody struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
	}
	require.NoError(t, json.Unmarshal(pdfRec.Body.Bytes(), &pdfBody))
	require.NotEmpty(t, pdfBody.URL)
	require.Equal(t, "application/pdf", pdfBody.MimeType)

	docxRec := getThreadDocument(t, router, token, threadID, "docx")
	require.Equal(t, http.StatusOK, docxRec.Code)
}
