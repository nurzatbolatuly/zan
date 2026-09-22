package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"zan-backend/internal/apierror"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/platform/ratelimit"
)

// traceIDHeader — заголовок сквозного trace_id между фронтом и backend
// (BACKEND_CODING_STANDARDS.md §4.4, backend-roadmap.md). Фронт передаёт
// его на каждый запрос; если запрос пришёл без заголовка (старая версия
// клиента, прямой curl) — backend генерирует новый и отдаёт его же в
// ответе, чтобы вызывающая сторона могла процитировать его в багрепорте.
const traceIDHeader = "X-Trace-Id"

// CORS — Stage 8 (BACKEND_PLAN.md §6 п.3 "Деплой": "CORS — со стороны
// бэкенда, не фронтенда"). allowedOrigins — точный список origin'ов
// (схема+хост+порт), не wildcard: сессия аутентифицируется httpOnly-cookie
// (session_middleware.go), а `Access-Control-Allow-Credentials: true` по
// спецификации CORS запрещено сочетать с `Access-Control-Allow-Origin: *`
// — нужен конкретный origin, эхом из запроса, если он есть в списке.
// Пустой allowedOrigins (ENV не задан) — CORS-заголовки не выставляются
// вообще: тот же самый ориджин (SPA и API за одним доменом/reverse-proxy)
// и любые non-browser клиенты (curl, мобильное приложение через
// Authorization-заголовок) при этом продолжают работать как раньше,
// браузер просто заблокирует кросс-ориджин fetch — безопасный дефолт, не
// открытый по ошибке.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}

		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Trace-Id, X-Admin-Token")
			c.Header("Access-Control-Max-Age", "600")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// RequestContext кладёт в context.Context запроса логгер, уже обогащённый
// trace_id — все нижележащие вызовы достают его через logger.FromContext
// и не собирают это поле заново (BACKEND_CODING_STANDARDS.md §4.1). Это
// первая и единственная точка, где trace_id читается из заголовка/
// генерируется — session_id добавится сюда же начиная со Stage 1, когда
// появится middleware сессии.
func RequestContext(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader(traceIDHeader)
		if traceID == "" {
			traceID = uuid.NewString()
		}
		c.Header(traceIDHeader, traceID)

		reqLogger := base.With(slog.String("trace_id", traceID))
		ctx := logger.WithContext(c.Request.Context(), reqLogger)
		ctx = logger.WithTraceID(ctx, traceID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// AccessLog логирует старт/итог каждого HTTP-запроса автоматически —
// хендлеру/сервису не нужно логировать сам факт похода в сеть руками
// (тот же принцип, что и у shared/lib/api.ts на фронте,
// BACKEND_CODING_STANDARDS.md §4.2).
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		l := logger.FromContext(c.Request.Context())

		c.Next()

		l.Info("http_request_completed", slog.Group("context",
			slog.String("method", c.Request.Method),
			slog.String("path", c.FullPath()),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("duration_ms", time.Since(started).Milliseconds()),
		))
	}
}

// Recovery ловит панику в хендлере, логирует её на уровне CRITICAL с полным
// стеком (backend-roadmap.md §6.4 — "инфраструктурный сбой уровня всего
// сервиса") и отвечает нейтральным 500, не отдавая клиенту сырой текст
// паники (BACKEND_CODING_STANDARDS.md §8 — пользователь не видит сырую
// ошибку интеграции).
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		l := logger.FromContext(c.Request.Context())
		l.Log(c.Request.Context(), logger.LevelCritical, "panic_recovered", slog.Group("context",
			slog.Any("recovered", recovered),
			slog.String("path", c.Request.URL.Path),
		))
		c.AbortWithStatus(500)
	})
}

// RateLimit — барьер против спама по IP (BACKEND_PLAN.md, Stage 1: "первый
// барьер против спама, до появления треда/оплаты, которые дороже"; Stage 7
// расширяет использование на POST /threads и POST /logs/client,
// zan-backend-tz-v3.md §5.7). store — общее состояние, собранное один раз в
// composition root (internal/platform/ratelimit), своё на каждый защищаемый
// роут (разные лимиты для разных по стоимости операций).
func RateLimit(store *ratelimit.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !store.Allow(c.ClientIP()) {
			writeError(c, rateLimitedError())
			return
		}
		c.Next()
	}
}

// RateLimitBySession — второй барьер на POST /threads, ключ — session_id,
// не IP (zan-backend-tz-v3.md §5.7: "rate limiting... по IP + по
// session_id"). Дополняет, не заменяет RateLimit: один IP может стоять за
// NAT/мобильной сетью с ротацией адресов, из-за чего IP-лимит для одной и
// той же сессии не срабатывает — session_id ловит именно этот случай.
// Монтируется ПОСЛЕ SessionAuth (нужен уже резолвленный session_id в
// контексте gin).
func RateLimitBySession(store *ratelimit.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !store.Allow(sessionFromGin(c).ID) {
			writeError(c, rateLimitedError())
			return
		}
		c.Next()
	}
}

func rateLimitedError() *apierror.Error {
	return &apierror.Error{
		Code:       "rate_limited",
		Message:    "Слишком много запросов, попробуйте чуть позже",
		HTTPStatus: http.StatusTooManyRequests,
	}
}
