package httpserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServices_List_ReturnsSeededActiveServices(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var services []struct {
		ID    string `json:"id"`
		Price int    `json:"price"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &services))
	require.Len(t, services, 2)
}

func TestAdmin_ListServices_WithoutToken_Returns401(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/services", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAdmin_UpdateService_ChangesPriceAndVisibility(t *testing.T) {
	router, _ := newTestRouter(t)

	body, err := json.Marshal(map[string]any{"price": 3500, "is_active": false})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/admin/services/qa", bytes.NewReader(body))
	req.Header.Set("X-Admin-Token", testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"price":3500`)
	require.Contains(t, rec.Body.String(), `"is_active":false`)

	// Деактивированная услуга пропадает из публичного списка.
	req = httptest.NewRequest(http.MethodGet, "/services", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var services []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &services))
	require.Len(t, services, 1)
	require.Equal(t, "doc", services[0].ID)
}

func TestAdmin_UpdateService_UnknownID_Returns404(t *testing.T) {
	router, _ := newTestRouter(t)

	body, err := json.Marshal(map[string]any{"price": 100, "is_active": true})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/admin/services/unknown", bytes.NewReader(body))
	req.Header.Set("X-Admin-Token", testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "service_not_found")
}

func createTestTariff(t *testing.T, router http.Handler, discountPercent int) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"name":             "Пакет вопросов",
		"discount_percent": discountPercent,
		"items":            []map[string]any{{"service_id": "qa", "qty": 3}},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/admin/tariffs", bytes.NewReader(body))
	req.Header.Set("X-Admin-Token", testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var tariff struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tariff))
	return tariff.ID
}

func TestAdmin_CreateTariff_ThenPublicListShowsComputedPrice(t *testing.T) {
	router, _ := newTestRouter(t)

	createTestTariff(t, router, 15)

	req := httptest.NewRequest(http.MethodGet, "/tariffs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var tariffs []struct {
		Subtotal int `json:"subtotal"`
		Total    int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tariffs))
	require.Len(t, tariffs, 1)
	require.Equal(t, 8700, tariffs[0].Subtotal)
	require.Equal(t, 7400, tariffs[0].Total)
}

func TestAdmin_CreateTariff_RejectsInvalidItems(t *testing.T) {
	router, _ := newTestRouter(t)

	body, err := json.Marshal(map[string]any{
		"name":             "Плохой пакет",
		"discount_percent": 10,
		"items":            []map[string]any{{"service_id": "qa", "qty": 21}},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/admin/tariffs", bytes.NewReader(body))
	req.Header.Set("X-Admin-Token", testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_request")
}

func TestAdmin_DeactivateTariff_RemovesFromPublicList(t *testing.T) {
	router, _ := newTestRouter(t)
	tariffID := createTestTariff(t, router, 15)

	req := httptest.NewRequest(http.MethodDelete, "/admin/tariffs/"+tariffID, nil)
	req.Header.Set("X-Admin-Token", testAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/tariffs", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, "[]", rec.Body.String())

	// В /admin/tariffs всё ещё виден (архив, не физическое удаление).
	req = httptest.NewRequest(http.MethodGet, "/admin/tariffs", nil)
	req.Header.Set("X-Admin-Token", testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), tariffID)
}
