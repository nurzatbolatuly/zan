package httpserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBalance_WithoutSession_Returns401(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/balance", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestBalance_StartsAtZeroForAllActiveServices(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	req := httptest.NewRequest(http.MethodGet, "/balance", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var balance []struct {
		ServiceID string `json:"service_id"`
		Quantity  int    `json:"quantity"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &balance))
	require.Len(t, balance, 2)
	for _, c := range balance {
		require.Zero(t, c.Quantity)
	}
}

func checkout(t *testing.T, router http.Handler, token string, payload map[string]any) (paymentID string, amount int, status int) {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/payments/checkout", bytes.NewReader(body))
	req.Header.Set("Authorization", "Session "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		return "", 0, rec.Code
	}
	var resp struct {
		PaymentID string `json:"payment_id"`
		Amount    int    `json:"amount"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.PaymentID, resp.Amount, rec.Code
}

func TestCheckoutAndConfirm_Custom_CreditsBalanceEndToEnd(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	paymentID, amount, status := checkout(t, router, token, map[string]any{
		"kind":  "custom",
		"items": []map[string]any{{"service_id": "qa", "qty": 2}, {"service_id": "doc", "qty": 1}},
	})
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, 10700, amount)

	req := httptest.NewRequest(http.MethodPost, "/payments/"+paymentID+"/confirm", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"success"`)

	req = httptest.NewRequest(http.MethodGet, "/balance", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Contains(t, rec.Body.String(), `"service_id":"qa","quantity":2`)
	require.Contains(t, rec.Body.String(), `"service_id":"doc","quantity":1`)
}

func TestCheckoutAndConfirm_Tariff_EndToEnd(t *testing.T) {
	router, _ := newTestRouter(t)
	tariffID := createTestTariff(t, router, 15)

	_, token, _ := createTestSession(t, router)
	paymentID, amount, status := checkout(t, router, token, map[string]any{
		"kind":      "tariff",
		"tariff_id": tariffID,
	})
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, 7400, amount) // qa*3=8700, -15% -> round to nearest 10

	req := httptest.NewRequest(http.MethodPost, "/payments/"+paymentID+"/confirm", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/balance", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Contains(t, rec.Body.String(), `"service_id":"qa","quantity":3`)
}

func TestConfirmPayment_IsIdempotent(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	paymentID, _, status := checkout(t, router, token, map[string]any{
		"kind":  "custom",
		"items": []map[string]any{{"service_id": "qa", "qty": 1}},
	})
	require.Equal(t, http.StatusCreated, status)

	confirm := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/payments/"+paymentID+"/confirm", nil)
		req.Header.Set("Authorization", "Session "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	require.Equal(t, http.StatusOK, confirm().Code)
	require.Equal(t, http.StatusOK, confirm().Code) // повторный confirm — не ошибка

	req := httptest.NewRequest(http.MethodGet, "/balance", nil)
	req.Header.Set("Authorization", "Session "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Contains(t, rec.Body.String(), `"service_id":"qa","quantity":1`) // не 2
}

func TestGetPayment_ForeignSession_Returns404(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)
	paymentID, _, status := checkout(t, router, token, map[string]any{
		"kind":  "custom",
		"items": []map[string]any{{"service_id": "qa", "qty": 1}},
	})
	require.Equal(t, http.StatusCreated, status)

	_, otherToken, _ := createTestSession(t, router)
	req := httptest.NewRequest(http.MethodGet, "/payments/"+paymentID, nil)
	req.Header.Set("Authorization", "Session "+otherToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "payment_not_found")
}

func TestCheckout_SingleService_RejectsMultipleItems(t *testing.T) {
	router, _ := newTestRouter(t)
	_, token, _ := createTestSession(t, router)

	_, _, status := checkout(t, router, token, map[string]any{
		"kind":  "single_service",
		"items": []map[string]any{{"service_id": "qa", "qty": 1}, {"service_id": "doc", "qty": 1}},
	})

	require.Equal(t, http.StatusBadRequest, status)
}
