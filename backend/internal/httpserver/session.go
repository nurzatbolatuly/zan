package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/domain"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/session"
)

// sessionResponse — {language, theme, onboarding_seen} из zan-backend-tz-v3.md
// §4 ("GET /sessions/me — текущая сессия") + session_id для удобства клиента.
type sessionResponse struct {
	SessionID      string  `json:"session_id"`
	Language       string  `json:"language"`
	Theme          *string `json:"theme,omitempty"`
	OnboardingSeen bool    `json:"onboarding_seen"`
}

func newSessionResponse(s domain.Session) sessionResponse {
	resp := sessionResponse{
		SessionID:      s.ID,
		Language:       string(s.Language),
		OnboardingSeen: s.OnboardingSeen,
	}
	if s.Theme != nil {
		theme := string(*s.Theme)
		resp.Theme = &theme
	}
	return resp
}

// createSessionResponse — ровно {session_token, session_id}
// (zan-backend-tz-v3.md §4, "POST /sessions").
type createSessionResponse struct {
	SessionToken string `json:"session_token"`
	SessionID    string `json:"session_id"`
}

// createSessionHandler строит POST /sessions.
func createSessionHandler(svc *session.Service, cookieSecure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess, token, err := svc.Create(c.Request.Context())
		if err != nil {
			logger.FromContext(c.Request.Context()).Error("session_create_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}

		setSessionCookie(c, token, cookieSecure)
		c.JSON(http.StatusCreated, createSessionResponse{
			SessionToken: token,
			SessionID:    sess.ID,
		})
	}
}

// getMeHandler — GET /sessions/me. Требует SessionAuth в группе маршрутов.
func getMeHandler(c *gin.Context) {
	c.JSON(http.StatusOK, newSessionResponse(sessionFromGin(c)))
}

// onboardingSeenHandler — POST /sessions/onboarding-seen.
func onboardingSeenHandler(svc *session.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFromGin(c)
		updated, err := svc.MarkOnboardingSeen(c.Request.Context(), sess.ID)
		if err != nil {
			logger.FromContext(c.Request.Context()).Error("session_mark_onboarding_seen_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}
		c.JSON(http.StatusOK, newSessionResponse(updated))
	}
}

// updateSessionRequest — PATCH /sessions/me: поля опциональны, nil = не
// трогать (zan-backend-tz-v3.md §4).
type updateSessionRequest struct {
	Language *string `json:"language"`
	Theme    *string `json:"theme"`
}

// updateMeHandler — PATCH /sessions/me.
func updateMeHandler(svc *session.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req updateSessionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, invalidRequestError("Invalid request body"))
			return
		}

		var language *domain.Language
		if req.Language != nil {
			lang, err := domain.ParseLanguage(*req.Language)
			if err != nil {
				writeError(c, invalidRequestError("Invalid language"))
				return
			}
			language = &lang
		}

		var theme *domain.Theme
		if req.Theme != nil {
			th, err := domain.ParseTheme(*req.Theme)
			if err != nil {
				writeError(c, invalidRequestError("Invalid theme"))
				return
			}
			theme = &th
		}

		sess := sessionFromGin(c)
		updated, err := svc.UpdatePreferences(c.Request.Context(), sess.ID, language, theme)
		if err != nil {
			logger.FromContext(c.Request.Context()).Error("session_update_preferences_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}
		c.JSON(http.StatusOK, newSessionResponse(updated))
	}
}
