package httpserver

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/domain"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/session"
)

const (
	sessionCookieName    = "session_token"
	sessionAuthzPrefix   = "Session "
	sessionGinContextKey = "domain_session"
)

// SessionAuth аутентифицирует запрос по opaque-токену сессии из
// Authorization: Session <token> (основной механизм фронта — instructions.md
// §6 "Транспорт сессии", bearer в localStorage) или cookie session_token
// (резервный путь) — zan-backend-tz-v3.md §2.1; для WS-хендшейка (GET
// /ws/threads/{id}, Stage 9) — ещё и query-параметр ?token=, см.
// extractSessionToken. Токен не найден, подделан или сессия истекла — 401
// без автосоздания (v3 §5.5, "как при первом заходе — чистое состояние").
// Продлевает скользящее окно на каждый успешный запрос
// (session.Service.Authenticate).
func SessionAuth(svc *session.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractSessionToken(c)
		if token == "" {
			writeError(c, sessionRequiredError())
			return
		}

		sess, err := svc.Authenticate(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, session.ErrInvalidToken) || errors.Is(err, session.ErrExpired) {
				writeError(c, sessionRequiredError())
				return
			}
			logger.FromContext(c.Request.Context()).Error("session_authenticate_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}

		l := logger.FromContext(c.Request.Context()).With(slog.String("session_id", sess.ID))
		c.Request = c.Request.WithContext(logger.WithContext(c.Request.Context(), l))
		c.Set(sessionGinContextKey, sess)
		c.Next()
	}
}

func sessionRequiredError() *apierror.Error {
	return &apierror.Error{
		Code:       "session_invalid",
		Message:    "Session is missing, invalid or expired",
		HTTPStatus: http.StatusUnauthorized,
	}
}

// extractSessionToken — заголовок/cookie (см. SessionAuth), плюс query-
// параметр ?token= как последний фолбэк (Stage 9): ТОЛЬКО ради WS-хендшейка
// (GET /ws/threads/{id}) — нативный браузерный WebSocket не умеет
// выставлять произвольные заголовки при апгрейде, а фронт держит токен
// сессии не в cookie, а в localStorage (instructions.md §6), поэтому
// cookie-путь для этого случая не подходит. На обычных REST-маршрутах этот
// фолбэк безвреден (невалидный/пустой токен в query всё равно даёт
// обычный 401) и не становится основным механизмом — bearer-заголовок им
// остаётся.
func extractSessionToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, sessionAuthzPrefix) {
		return strings.TrimPrefix(h, sessionAuthzPrefix)
	}
	if cookie, err := c.Cookie(sessionCookieName); err == nil {
		return cookie
	}
	return c.Query("token")
}

// sessionFromGin достаёт сессию, положенную SessionAuth. Паникует, если
// вызвана вне группы маршрутов с этим middleware — программная ошибка
// роутинга, а не штатный сценарий (BACKEND_CODING_STANDARDS.md §6.1: panic
// допустим для ошибок, пойманных на старте/в структуре кода).
func sessionFromGin(c *gin.Context) domain.Session {
	return c.MustGet(sessionGinContextKey).(domain.Session)
}

// setSessionCookie кладёт токен в httpOnly-cookie для веба (v3 §2.1).
// secure=false только в dev (локальный HTTP без TLS) — в проде всегда true,
// см. NewRouter/cmd/api, где секьюрность выводится из config.Env.
func setSessionCookie(c *gin.Context, token string, secure bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, token, int(session.TTL.Seconds()), "/", "", secure, true)
}
