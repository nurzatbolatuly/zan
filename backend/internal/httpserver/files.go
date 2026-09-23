package httpserver

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/domain"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/file"
)

// fileAttachmentResponse — {file_id, url} на upload, метаданные+url на
// GET /files/{id} (zan-backend-tz-v2.md §3.3, §2.8).
type fileAttachmentResponse struct {
	FileID           string `json:"file_id"`
	URL              string `json:"url"`
	OriginalName     string `json:"original_name"`
	MimeType         string `json:"mime_type"`
	SizeBytes        int64  `json:"size_bytes"`
	ProcessingStatus string `json:"processing_status"`
}

func newFileAttachmentResponse(f domain.FileAttachment, url string) fileAttachmentResponse {
	return fileAttachmentResponse{
		FileID:           f.ID,
		URL:              url,
		OriginalName:     f.OriginalName,
		MimeType:         f.MimeType,
		SizeBytes:        f.SizeBytes,
		ProcessingStatus: string(f.ProcessingStatus),
	}
}

// uploadFileHandler — POST /files/upload (zan-backend-tz-v2.md §3.3):
// multipart-поле "file". Требует SessionAuth. maxSizeBytes — читается из
// запроса до полного разбора multipart-тела (MaxBytesReader), чтобы
// заведомо большой upload не тратил память на чтение целиком до проверки
// размера — file.Service всё равно перепроверяет len(data) сам, это
// защита раньше по стеку, не дублирование ради дублирования.
func uploadFileHandler(svc *file.Service, maxSizeBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// MaxBytesReader ловит переполнение уже на этапе разбора multipart-тела
		// (c.FormFile ниже вызывает ParseMultipartForm) — *http.MaxBytesError
		// отличает "тело больше лимита" от прочих ошибок парсинга.
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSizeBytes+1)

		fh, err := c.FormFile("file")
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeError(c, fileTooLargeError(maxSizeBytes))
				return
			}
			writeError(c, invalidRequestError("Missing multipart field \"file\""))
			return
		}

		opened, err := fh.Open()
		if err != nil {
			writeError(c, apierror.Internal())
			return
		}
		data, err := io.ReadAll(opened)
		_ = opened.Close()
		if err != nil {
			writeError(c, apierror.Internal())
			return
		}

		mimeType := fh.Header.Get("Content-Type")
		sess := sessionFromGin(c)
		created, url, err := svc.Upload(c.Request.Context(), file.UploadRequest{
			SessionID:    sess.ID,
			OriginalName: fh.Filename,
			MimeType:     mimeType,
			Data:         data,
		})
		if err != nil {
			writeFileError(c, err, maxSizeBytes)
			return
		}
		c.JSON(http.StatusCreated, newFileAttachmentResponse(created, url))
	}
}

// getFileHandler — GET /files/{id}: метаданные + свежая ссылка на скачивание
// (zan-backend-tz-v2.md §3.3). Требует SessionAuth.
func getFileHandler(svc *file.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFromGin(c)
		f, url, err := svc.GetByID(c.Request.Context(), sess.ID, c.Param("id"))
		if err != nil {
			writeFileError(c, err, 0)
			return
		}
		c.JSON(http.StatusOK, newFileAttachmentResponse(f, url))
	}
}

// fileTooLargeError — единая точка формулировки "файл слишком большой",
// используется и на multipart-переполнении (http.MaxBytesReader), и на
// собственной проверке file.Service.Upload (file.ErrTooLarge).
func fileTooLargeError(maxSizeBytes int64) *apierror.Error {
	return &apierror.Error{
		Code:       "file_too_large",
		Message:    fmt.Sprintf("Файл слишком большой (макс. %d МБ)", maxSizeBytes/(1024*1024)),
		HTTPStatus: http.StatusRequestEntityTooLarge,
	}
}

func writeFileError(c *gin.Context, err error, maxSizeBytes int64) {
	switch {
	case errors.Is(err, file.ErrNotFound):
		writeError(c, notFoundError("file_not_found", "Файл не найден"))
	case errors.Is(err, file.ErrTooLarge):
		writeError(c, fileTooLargeError(maxSizeBytes))
	case errors.Is(err, file.ErrRejectedByAVScanner):
		writeError(c, &apierror.Error{
			Code:       "file_rejected",
			Message:    "Не удалось загрузить файл",
			HTTPStatus: http.StatusUnprocessableEntity,
		})
	default:
		logger.FromContext(c.Request.Context()).Error("file_operation_failed",
			slog.Group("context", slog.String("error", err.Error())))
		writeError(c, apierror.Internal())
	}
}
