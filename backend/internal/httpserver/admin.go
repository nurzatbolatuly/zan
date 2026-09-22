package httpserver

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/platform/bruteforce"
)

const adminTokenHeader = "X-Admin-Token"

// AdminAuth — гейт на /admin/* по статичному секрету, не завязанному на
// сессии пользователей (zan-backend-tz-v3.md §2.3, BACKEND_PLAN.md §6 п.1).
// Сравнение — constant-time (subtle.ConstantTimeCompare), чтобы не течь
// длиной/содержимым секрета через тайминг ответа.
//
// bf — блокировка IP после N неудачных попыток подряд (zan-backend-tz-v3.md
// §5.7, Stage 7): проверяется ПЕРЕД сравнением токена — заблокированный IP
// получает 429 и не может даже проверить очередную догадку, не только
// "долго ждёт между попытками", как дал бы один RateLimit.
func AdminAuth(adminToken string, bf *bruteforce.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !bf.Allowed(ip) {
			writeError(c, &apierror.Error{
				Code:       "admin_blocked",
				Message:    "Too many failed attempts, try again later",
				HTTPStatus: http.StatusTooManyRequests,
			})
			return
		}

		got := c.GetHeader(adminTokenHeader)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(adminToken)) != 1 {
			bf.RecordFailure(ip)
			writeError(c, &apierror.Error{
				Code:       "admin_unauthorized",
				Message:    "Invalid or missing admin token",
				HTTPStatus: http.StatusUnauthorized,
			})
			return
		}
		bf.RecordSuccess(ip)
		c.Next()
	}
}

// adminPingHandler — минимальная ручка под /admin/*, существует только
// чтобы AdminAuth было на чём проверить (Stage 1 DoD: "X-Admin-Token-гейт
// возвращает 401 без токена и 200 с верным") — до Stage 2/7, когда
// появятся настоящие /admin/services, /admin/tariffs, /admin/prompts,
// /admin/analytics/overview.
func adminPingHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
