package httpserver

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/voice"
)

// transcribeResponse — {text} (zan-backend-tz-v2.md §3.3).
type transcribeResponse struct {
	Text string `json:"text"`
}

// transcribeVoiceHandler — POST /voice/transcribe (zan-backend-tz-v2.md §4.7):
// multipart-поле "audio". Требует SessionAuth. Синхронный вызов — клиент
// получает текст до отправки сообщения в тред, ничего не сохраняется в БД.
func transcribeVoiceHandler(svc *voice.Service, maxSizeBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		upload, apiErr := readUploadedFile(c, "audio", maxSizeBytes)
		if apiErr != nil {
			writeError(c, apiErr)
			return
		}
		if upload == nil {
			writeError(c, invalidRequestError("Missing multipart field \"audio\""))
			return
		}

		sess := sessionFromGin(c)
		text, err := svc.Transcribe(c.Request.Context(), voice.TranscribeRequest{
			SessionID: sess.ID,
			MimeType:  upload.ContentType,
			Lang:      sess.Language,
			Data:      upload.Data,
		})
		if err != nil {
			writeVoiceError(c, err, maxSizeBytes)
			return
		}
		c.JSON(http.StatusOK, transcribeResponse{Text: text})
	}
}

func writeVoiceError(c *gin.Context, err error, maxSizeBytes int64) {
	switch {
	case errors.Is(err, voice.ErrUnsupportedMimeType):
		writeError(c, &apierror.Error{
			Code:       "unsupported_file_type",
			Message:    "Этот формат аудио не поддерживается",
			HTTPStatus: http.StatusUnsupportedMediaType,
		})
	case errors.Is(err, voice.ErrTooLarge):
		writeError(c, fileTooLargeError(maxSizeBytes))
	case errors.Is(err, voice.ErrEmptyTranscript):
		writeError(c, &apierror.Error{
			Code:       "empty_transcript",
			Message:    "Не расслышал, повторите ещё раз",
			HTTPStatus: http.StatusUnprocessableEntity,
		})
	default:
		logger.FromContext(c.Request.Context()).Error("voice_transcribe_failed",
			slog.Group("context", slog.String("error", err.Error())))
		writeError(c, &apierror.Error{
			Code:       "voice_transcribe_failed",
			Message:    "Не удалось распознать голос, попробуйте текстом",
			HTTPStatus: http.StatusServiceUnavailable,
		})
	}
}
