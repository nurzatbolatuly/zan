package httpserver

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/billing"
	"zan-backend/internal/service/catalog"
	"zan-backend/internal/service/document"
)

// generateDocumentResponse — ответ POST /threads/{id}/generate-document:
// сообщение ассистента (та же форма, что и остальные ответы треда) +
// files_ready. files_ready=false — текст уже сохранён и виден в
// GET /threads/{id}, но DocumentsService.Render не смог подготовить файл
// (backend-roadmap.md §5.2, последняя строка) — не HTTP-ошибка, клиент
// должен предложить пользователю попробовать генерацию ещё раз
// ("Ответ готов, но не удалось подготовить файл документа. Попробовать ещё раз").
type generateDocumentResponse struct {
	messageResponse
	FilesReady bool `json:"files_ready"`
}

// generateDocumentHandler — POST /threads/{id}/generate-document. Требует SessionAuth.
func generateDocumentHandler(svc *document.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFromGin(c)
		msg, filesReady, err := svc.Generate(c.Request.Context(), c.Param("id"), sess.ID)
		if err != nil {
			writeDocumentError(c, err)
			return
		}
		c.JSON(http.StatusCreated, generateDocumentResponse{
			messageResponse: newMessageResponse(msg),
			FilesReady:      filesReady,
		})
	}
}

// getThreadDocumentHandler — GET /threads/{id}/document?format=pdf|docx.
// Требует SessionAuth. Ответ — та же форма, что GET /files/{id}
// (fileAttachmentResponse): свежая presigned-ссылка, не хранится статично.
func getThreadDocumentHandler(svc *document.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFromGin(c)
		f, url, err := svc.GetDocument(c.Request.Context(), c.Param("id"), sess.ID, c.Query("format"))
		if err != nil {
			writeDocumentError(c, err)
			return
		}
		c.JSON(http.StatusOK, newFileAttachmentResponse(f, url))
	}
}

func writeDocumentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, document.ErrThreadNotFound):
		writeError(c, notFoundError("thread_not_found", "Thread not found"))
	case errors.Is(err, catalog.ErrServiceNotFound):
		writeError(c, notFoundError("service_not_found", "Service not found"))
	case errors.Is(err, document.ErrInvalidFormat):
		writeError(c, invalidRequestError("format must be one of: pdf, docx"))
	case errors.Is(err, document.ErrNotGenerated):
		writeError(c, notFoundError("document_not_generated", "Документ ещё не сгенерирован в этом формате"))
	case errors.Is(err, billing.ErrInsufficientBalance):
		writeError(c, &apierror.Error{
			Code:       "payment_required",
			Message:    "Для генерации документа нужно оплатить услугу",
			HTTPStatus: http.StatusConflict,
		})
	case errors.Is(err, document.ErrGenerationFailed):
		writeError(c, &apierror.Error{
			Code:       "document_generation_failed",
			Message:    "Не удалось обработать запрос, попробуйте ещё раз. Оплата возвращена на баланс",
			HTTPStatus: http.StatusBadGateway,
		})
	default:
		logger.FromContext(c.Request.Context()).Error("document_operation_failed",
			slog.Group("context", slog.String("error", err.Error())))
		writeError(c, apierror.Internal())
	}
}
