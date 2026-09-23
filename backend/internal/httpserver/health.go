package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// healthzResponse — минимальный контракт liveness-пробы для
// оркестратора/Docker HEALTHCHECK (BACKEND_PLAN.md, Stage 0 DoD). Не публичный
// API-эндпоинт из openapi.yaml — сюда не добавляются проверки зависимостей
// (БД/S3/helper), это чистый "процесс жив", не "готов обслуживать трафик".
type healthzResponse struct {
	Status string `json:"status"`
}

func healthzHandler(c *gin.Context) {
	c.JSON(http.StatusOK, healthzResponse{Status: "ok"})
}
