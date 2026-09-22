package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
)

// writeError отвечает клиенту {code, message} с нужным HTTP-статусом и
// прерывает цепочку хендлеров (BACKEND_CODING_STANDARDS.md §8) — единая
// точка, чтобы формат ошибки не расходился между хендлерами.
func writeError(c *gin.Context, err *apierror.Error) {
	c.AbortWithStatusJSON(err.HTTPStatus, err)
}

// invalidRequestError — общий 400 для невалидного тела/параметров запроса,
// переиспользуется всеми хендлерами (изначально жил только в session.go —
// Stage 2 сделал его вторым потребителем, BACKEND_CODING_STANDARDS.md §3).
func invalidRequestError(message string) *apierror.Error {
	return &apierror.Error{
		Code:       "invalid_request",
		Message:    message,
		HTTPStatus: http.StatusBadRequest,
	}
}

// notFoundError — общий 404 с конкретным машиночитаемым кодом (например,
// "service_not_found", "tariff_not_found", "payment_not_found" —
// BACKEND_PLAN.md §6 п.6).
func notFoundError(code, message string) *apierror.Error {
	return &apierror.Error{
		Code:       code,
		Message:    message,
		HTTPStatus: http.StatusNotFound,
	}
}
