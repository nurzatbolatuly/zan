package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/domain"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/analytics"
)

// analyticsOverviewResponse — GET /admin/analytics/overview
// (zan-backend-tz-v2.md §3.8). avg_processing_time_sec/satisfaction_rate —
// omitempty через указатель: отсутствуют в JSON, если ни одного отвеченного
// сообщения/оценки ещё нет, не притворяются нулём (см. domain.AnalyticsOverview).
type analyticsOverviewResponse struct {
	TotalThreads         int            `json:"total_threads"`
	StatusBreakdown      map[string]int `json:"status_breakdown"`
	AvgProcessingTimeSec *float64       `json:"avg_processing_time_sec,omitempty"`
	SatisfactionRate     *float64       `json:"satisfaction_rate,omitempty"`
}

// allThreadStatuses — фиксированный порядок enum ThreadStatus, чтобы
// status_breakdown всегда нёс все известные статусы с count=0, а не только
// те, что реально встретились (клиенту не нужно самому знать полный список
// возможных ключей, чтобы нарисовать разбивку без "дыр").
var allThreadStatuses = []domain.ThreadStatus{
	domain.ThreadStatusAwaitingPayment,
	domain.ThreadStatusProcessing,
	domain.ThreadStatusDone,
	domain.ThreadStatusError,
	domain.ThreadStatusCanceled,
}

func newAnalyticsOverviewResponse(o domain.AnalyticsOverview) analyticsOverviewResponse {
	breakdown := make(map[string]int, len(allThreadStatuses))
	for _, status := range allThreadStatuses {
		breakdown[string(status)] = o.StatusBreakdown[status]
	}
	return analyticsOverviewResponse{
		TotalThreads:         o.TotalThreads,
		StatusBreakdown:      breakdown,
		AvgProcessingTimeSec: o.AvgProcessingTimeSec,
		SatisfactionRate:     o.SatisfactionRate,
	}
}

// adminAnalyticsOverviewHandler — GET /admin/analytics/overview. Требует AdminAuth.
func adminAnalyticsOverviewHandler(svc *analytics.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		overview, err := svc.GetOverview(c.Request.Context())
		if err != nil {
			logger.FromContext(c.Request.Context()).Error("analytics_overview_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}
		c.JSON(http.StatusOK, newAnalyticsOverviewResponse(overview))
	}
}
