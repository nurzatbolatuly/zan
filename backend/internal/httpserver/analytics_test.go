package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminAnalyticsOverview_WithoutToken_Returns401(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/analytics/overview", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAdminAnalyticsOverview_WithToken_ReturnsFullStatusBreakdown(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/analytics/overview", nil)
	req.Header.Set("X-Admin-Token", testAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		TotalThreads         int            `json:"total_threads"`
		StatusBreakdown      map[string]int `json:"status_breakdown"`
		AvgProcessingTimeSec *float64       `json:"avg_processing_time_sec"`
		SatisfactionRate     *float64       `json:"satisfaction_rate"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.TotalThreads)
	// Все 5 статусов присутствуют явно с 0, а не только те, что встретились
	// (newFakeAnalyticsRepo по умолчанию отдаёт пустой AnalyticsOverview).
	require.Len(t, body.StatusBreakdown, 5)
	for _, status := range []string{"awaiting_payment", "processing", "done", "error", "canceled"} {
		require.Contains(t, body.StatusBreakdown, status)
	}
	// nil-указатели без данных сериализуются как отсутствие поля (omitempty).
	require.Nil(t, body.AvgProcessingTimeSec)
	require.Nil(t, body.SatisfactionRate)
}
