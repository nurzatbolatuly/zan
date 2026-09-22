// Package httpserver — тонкий слой публичного REST/JSON API
// (BACKEND_CODING_STANDARDS.md §1.1): парсинг запроса, вызов service,
// маппинг результата/ошибки в HTTP-ответ. Бизнес-правила сюда не попадают.
package httpserver

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/platform/bruteforce"
	"zan-backend/internal/platform/ratelimit"
	"zan-backend/internal/service/analytics"
	"zan-backend/internal/service/billing"
	"zan-backend/internal/service/catalog"
	"zan-backend/internal/service/clientlog"
	"zan-backend/internal/service/document"
	"zan-backend/internal/service/file"
	"zan-backend/internal/service/prompt"
	"zan-backend/internal/service/session"
	"zan-backend/internal/service/thread"
	"zan-backend/internal/service/voice"
)

// Deps — зависимости, которые NewRouter пробрасывает хендлерам. Собираются
// вручную в cmd/api (composition root, без DI-фреймворка —
// BACKEND_CODING_STANDARDS.md §1.1/§2) и растут по мере стадий
// (BACKEND_PLAN.md) — Stage 4 добавляет файлы/голос.
type Deps struct {
	Sessions            *session.Service
	Catalog             *catalog.Service
	Billing             *billing.Service
	Thread              *thread.Service
	Documents           *document.Service
	Files               *file.Service
	Voice               *voice.Service
	Prompts             *prompt.Service
	Analytics           *analytics.Service
	ClientLogs          *clientlog.Service
	FileMaxSizeBytes    int64
	VoiceMaxSizeBytes   int64
	AdminToken          string
	SessionCookieSecure bool
	SessionRateLimit    *ratelimit.Store
	// ThreadRateLimit/ThreadSessionRateLimit — Stage 7, оба барьера на
	// POST /threads (zan-backend-tz-v3.md §5.7: "по IP + по session_id"),
	// см. RateLimit/RateLimitBySession.
	ThreadRateLimit        *ratelimit.Store
	ThreadSessionRateLimit *ratelimit.Store
	// ClientLogRateLimit — Stage 7, POST /logs/client не требует сессии,
	// единственная защита от спама — IP (zan-backend-tz-v3.md §6.3).
	ClientLogRateLimit *ratelimit.Store
	// AdminBruteforce — Stage 7, блокировка IP после N неудачных попыток
	// на /admin/* (zan-backend-tz-v3.md §5.7).
	AdminBruteforce *bruteforce.Store
	// CORSAllowedOrigins — Stage 8, см. CORS() в middleware.go. Пусто —
	// CORS-заголовки не выставляются (см. комментарий у CORS()).
	CORSAllowedOrigins []string
}

// NewRouter собирает gin.Engine с нуля (gin.New(), не gin.Default()) —
// стандартные gin-логгер/recovery заменены на свои (AccessLog/Recovery),
// говорящие в контракте backend-roadmap.md §6.1, а не в формате gin по
// умолчанию.
func NewRouter(baseLogger *slog.Logger, deps Deps) *gin.Engine {
	r := gin.New()
	r.Use(RequestContext(baseLogger), AccessLog(), Recovery(), CORS(deps.CORSAllowedOrigins))

	r.GET("/healthz", healthzHandler)

	r.POST("/sessions", RateLimit(deps.SessionRateLimit), createSessionHandler(deps.Sessions, deps.SessionCookieSecure))

	authed := r.Group("/sessions")
	authed.Use(SessionAuth(deps.Sessions))
	authed.GET("/me", getMeHandler)
	authed.PATCH("/me", updateMeHandler(deps.Sessions))
	authed.POST("/onboarding-seen", onboardingSeenHandler(deps.Sessions))

	// Каталог (zan-backend-tz-v2.md §3.4) — публичный, без сессии.
	r.GET("/services", listServicesHandler(deps.Catalog))
	r.GET("/tariffs", listTariffsHandler(deps.Catalog))

	// Баланс/оплата (§3.4/§3.5) — требуют сессии, та же группа, что /sessions/me.
	billingAuthed := r.Group("/", SessionAuth(deps.Sessions))
	billingAuthed.GET("/balance", getBalanceHandler(deps.Billing))
	billingAuthed.POST("/payments/checkout", checkoutHandler(deps.Billing))
	billingAuthed.GET("/payments/:id", getPaymentHandler(deps.Billing))
	billingAuthed.POST("/payments/:id/confirm", confirmPaymentHandler(deps.Billing, deps.Thread))

	// Треды/сообщения (zan-backend-tz-v2.md §3.2/§3.6, Stage 3) — та же
	// сессионная группа маршрутов.
	threadAuthed := r.Group("/", SessionAuth(deps.Sessions))
	// Stage 7 (zan-backend-tz-v3.md §5.7) — единственный роут этой группы с
	// собственным rate limiting: создание треда списывает баланс/вызывает
	// LLM, самая дорогая операция публичного API, требует барьера и по IP,
	// и по session_id (см. Deps.ThreadRateLimit/ThreadSessionRateLimit).
	threadAuthed.POST("/threads", RateLimit(deps.ThreadRateLimit), RateLimitBySession(deps.ThreadSessionRateLimit), createThreadHandler(deps.Thread))
	threadAuthed.GET("/threads", listThreadsHandler(deps.Thread))
	threadAuthed.GET("/threads/:id", getThreadHandler(deps.Thread))
	threadAuthed.POST("/threads/:id/messages", addMessageHandler(deps.Thread))
	threadAuthed.POST("/threads/:id/cancel", cancelThreadHandler(deps.Thread))
	threadAuthed.DELETE("/threads/:id", deleteThreadHandler(deps.Thread))
	threadAuthed.POST("/messages/:id/feedback", messageFeedbackHandler(deps.Thread))

	// Агент "Документы" (zan-backend-tz-v2.md §3.2/§4.4, Stage 6) — та же
	// сессионная группа маршрутов.
	threadAuthed.POST("/threads/:id/generate-document", generateDocumentHandler(deps.Documents))
	threadAuthed.GET("/threads/:id/document", getThreadDocumentHandler(deps.Documents))

	// Файлы/голос (zan-backend-tz-v2.md §3.3, Stage 4) — та же сессионная группа.
	filesAuthed := r.Group("/", SessionAuth(deps.Sessions))
	filesAuthed.POST("/files/upload", uploadFileHandler(deps.Files, deps.FileMaxSizeBytes))
	filesAuthed.GET("/files/:id", getFileHandler(deps.Files))
	filesAuthed.POST("/voice/transcribe", transcribeVoiceHandler(deps.Voice, deps.VoiceMaxSizeBytes))

	// Логи с фронта (zan-backend-tz-v3.md §6.3, Stage 7) — без сессии
	// намеренно (событие может произойти до того, как сессия вообще
	// появилась), единственный барьер — rate limiting по IP.
	r.POST("/logs/client", RateLimit(deps.ClientLogRateLimit), postClientLogHandler(deps.ClientLogs))

	admin := r.Group("/admin")
	admin.Use(AdminAuth(deps.AdminToken, deps.AdminBruteforce))
	admin.GET("/ping", adminPingHandler)
	admin.GET("/services", adminListServicesHandler(deps.Catalog))
	admin.PUT("/services/:id", adminUpdateServiceHandler(deps.Catalog))
	admin.GET("/tariffs", adminListTariffsHandler(deps.Catalog))
	admin.POST("/tariffs", adminCreateTariffHandler(deps.Catalog))
	admin.PUT("/tariffs/:id", adminUpdateTariffHandler(deps.Catalog))
	admin.DELETE("/tariffs/:id", adminDeactivateTariffHandler(deps.Catalog))
	// Промпты агентов (zan-backend-tz-v2.md §3.7, Stage 5) — internal/agent
	// читает qa через prompt.Service.GetPromptText на каждый вызов LLM, эти
	// два роута — единственный способ поменять текст без деплоя кода.
	admin.GET("/prompts", adminListPromptsHandler(deps.Prompts))
	admin.PUT("/prompts/:agent_type", adminUpdatePromptHandler(deps.Prompts))
	// Аналитика (zan-backend-tz-v2.md §3.8, Stage 7).
	admin.GET("/analytics/overview", adminAnalyticsOverviewHandler(deps.Analytics))

	return r
}
