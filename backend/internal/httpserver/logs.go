package httpserver

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/service/clientlog"
)

// postClientLogRequest — POST /logs/client (zan-backend-tz-v3.md §6.3).
type postClientLogRequest struct {
	SessionID string         `json:"session_id"`
	TraceID   string         `json:"trace_id"`
	Level     string         `json:"level" binding:"required"`
	Event     string         `json:"event" binding:"required"`
	Message   string         `json:"message" binding:"required"`
	Context   map[string]any `json:"context"`
}

// postClientLogHandler — POST /logs/client. Без SessionAuth (v3 §6.3:
// событие может произойти ДО того, как сессия вообще создалась — например,
// сам POST /sessions упал) — единственная защита от злоупотребления здесь
// это RateLimit по IP (см. router.go), не авторизация.
func postClientLogHandler(svc *clientlog.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req postClientLogRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, invalidRequestError("Invalid request body"))
			return
		}
		level, err := clientlog.ParseLevel(req.Level)
		if err != nil {
			writeError(c, invalidRequestError("Invalid level"))
			return
		}

		err = svc.Log(c.Request.Context(), clientlog.Entry{
			SessionID: req.SessionID,
			TraceID:   req.TraceID,
			Level:     level,
			Event:     req.Event,
			Message:   req.Message,
			Context:   req.Context,
		})
		if err != nil {
			if errors.Is(err, clientlog.ErrEmptyEvent) || errors.Is(err, clientlog.ErrEmptyMessage) {
				writeError(c, invalidRequestError("event and message must not be empty"))
				return
			}
			writeError(c, apierror.Internal())
			return
		}
		c.Status(http.StatusNoContent)
	}
}
