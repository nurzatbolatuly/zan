package httpserver

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/domain"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/billing"
	"zan-backend/internal/service/catalog"
	"zan-backend/internal/service/file"
	"zan-backend/internal/service/thread"
)

// rfc3339Format — единый формат сериализации timestamp-полей в JSON-ответах
// (тот же формат, что уже использует newPaymentResponse в billing.go).
const rfc3339Format = "2006-01-02T15:04:05Z07:00"

// sourceResponse/findingResponse — {ref, quote}/{title, body}
// (zan-backend-tz-v2.md §2.7).
type sourceResponse struct {
	Ref   string `json:"ref"`
	Quote string `json:"quote"`
}

type findingResponse struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// messageResponse — Message (zan-backend-tz-v2.md §2.7).
// UnverifiedSources — Stage 5 (backend-roadmap.md §6 открытый вопрос №11):
// бэк отдаёт флаг в любом случае, решение "показывать ли" в UI — за
// фронтом. Без omitempty (как is_paid у threadResponse) — false для
// пользовательских сообщений и валидных ответов ассистента одинаково
// значимо, не "поле отсутствует".
type messageResponse struct {
	ID                string            `json:"id"`
	Sender            string            `json:"sender"`
	InputType         string            `json:"input_type"`
	Text              string            `json:"text"`
	Sources           []sourceResponse  `json:"sources,omitempty"`
	Findings          []findingResponse `json:"findings,omitempty"`
	UnverifiedSources bool              `json:"unverified_sources"`
	Feedback          *string           `json:"feedback,omitempty"`
	ProcessingTimeMs  *int              `json:"processing_time_ms,omitempty"`
	CreatedAt         string            `json:"created_at"`
}

func newMessageResponse(m domain.Message) messageResponse {
	resp := messageResponse{
		ID:                m.ID,
		Sender:            string(m.Sender),
		InputType:         string(m.InputType),
		Text:              m.Text,
		UnverifiedSources: m.UnverifiedSources,
		ProcessingTimeMs:  m.ProcessingTimeMs,
		CreatedAt:         formatTime(m.CreatedAt),
	}
	for _, src := range m.Sources {
		resp.Sources = append(resp.Sources, sourceResponse{Ref: src.Ref, Quote: src.Quote})
	}
	for _, f := range m.Findings {
		resp.Findings = append(resp.Findings, findingResponse{Title: f.Title, Body: f.Body})
	}
	if m.Feedback != nil {
		v := string(*m.Feedback)
		resp.Feedback = &v
	}
	return resp
}

// threadResponse — Thread без сообщений, используется в списке GET /threads
// (zan-backend-tz-v2.md §2.6).
type threadResponse struct {
	ID            string  `json:"id"`
	ServiceID     string  `json:"service_id"`
	Status        string  `json:"status"`
	Title         string  `json:"title"`
	PreviewText   string  `json:"preview_text"`
	MessageCount  int     `json:"message_count"`
	IsPaid        bool    `json:"is_paid"`
	PaidAt        *string `json:"paid_at,omitempty"`
	FreeUntil     *string `json:"free_until,omitempty"`
	CreatedAt     string  `json:"created_at"`
	LastMessageAt *string `json:"last_message_at,omitempty"`
	ClosedAt      *string `json:"closed_at,omitempty"`
}

func newThreadResponse(t domain.Thread) threadResponse {
	return threadResponse{
		ID:            t.ID,
		ServiceID:     t.ServiceID,
		Status:        string(t.Status),
		Title:         t.Title,
		PreviewText:   t.PreviewText,
		MessageCount:  t.MessageCount,
		IsPaid:        t.IsPaid,
		PaidAt:        formatTimePtr(t.PaidAt),
		FreeUntil:     formatTimePtr(t.FreeUntil),
		CreatedAt:     formatTime(t.CreatedAt),
		LastMessageAt: formatTimePtr(t.LastMessageAt),
		ClosedAt:      formatTimePtr(t.ClosedAt),
	}
}

// threadDetailResponse — Thread + сообщения, GET /threads/{id} и ответ
// POST /threads, POST /threads/{id}/messages (синхронная заглушка агента —
// ответ ассистента уже готов к моменту ответа на запрос, Stage 3).
type threadDetailResponse struct {
	threadResponse
	Messages []messageResponse `json:"messages"`
}

func newThreadDetailResponse(t domain.Thread, msgs []domain.Message) threadDetailResponse {
	out := make([]messageResponse, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, newMessageResponse(m))
	}
	return threadDetailResponse{threadResponse: newThreadResponse(t), Messages: out}
}

func formatTime(t time.Time) string {
	return t.Format(rfc3339Format)
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := t.Format(rfc3339Format)
	return &v
}

// createThreadRequest — POST /threads (zan-backend-tz-v2.md §3.2:
// "{service_id, text, input_type, file_ids?}"). Text — не binding:"required"
// начиная со Stage 4: сообщение только с вложением (input_type=file) не
// несёт текста, thread.Service сам проверяет "текст ИЛИ файл" (см.
// validateMessageInput).
type createThreadRequest struct {
	ServiceID string   `json:"service_id" binding:"required"`
	Text      string   `json:"text"`
	InputType string   `json:"input_type" binding:"required"`
	FileIDs   []string `json:"file_ids"`
}

// createThreadHandler — POST /threads. Требует SessionAuth.
func createThreadHandler(svc *thread.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createThreadRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, invalidRequestError("Invalid request body"))
			return
		}
		inputType, err := domain.ParseMessageInputType(req.InputType)
		if err != nil {
			writeError(c, invalidRequestError("Invalid input_type"))
			return
		}

		sess := sessionFromGin(c)
		t, err := svc.CreateThread(c.Request.Context(), thread.CreateThreadRequest{
			SessionID: sess.ID,
			Language:  sess.Language,
			ServiceID: req.ServiceID,
			Text:      req.Text,
			InputType: inputType,
			FileIDs:   req.FileIDs,
		})
		if err != nil {
			writeThreadError(c, err)
			return
		}

		msgs, err := loadMessagesOrFail(c, svc, t.ID, sess.ID)
		if err != nil {
			return
		}
		c.JSON(http.StatusCreated, newThreadDetailResponse(t, msgs))
	}
}

// listThreadsHandler — GET /threads?status=&search=&page=. Требует SessionAuth.
func listThreadsHandler(svc *thread.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := 1
		if raw := c.Query("page"); raw != "" {
			p, err := strconv.Atoi(raw)
			if err != nil || p < 1 {
				writeError(c, invalidRequestError("Invalid page"))
				return
			}
			page = p
		}

		sess := sessionFromGin(c)
		items, total, err := svc.ListThreads(c.Request.Context(), sess.ID, c.Query("status"), c.Query("search"), page)
		if err != nil {
			writeThreadError(c, err)
			return
		}

		out := make([]threadResponse, 0, len(items))
		for _, t := range items {
			out = append(out, newThreadResponse(t))
		}
		c.JSON(http.StatusOK, gin.H{
			"items":     out,
			"page":      page,
			"page_size": thread.ListPageSize,
			"total":     total,
		})
	}
}

// getThreadHandler — GET /threads/{id}. Требует SessionAuth.
func getThreadHandler(svc *thread.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFromGin(c)
		t, msgs, err := svc.GetThread(c.Request.Context(), c.Param("id"), sess.ID)
		if err != nil {
			writeThreadError(c, err)
			return
		}
		c.JSON(http.StatusOK, newThreadDetailResponse(t, msgs))
	}
}

// addMessageRequest — POST /threads/{id}/messages (zan-backend-tz-v2.md §3.2:
// "{text, input_type, file_ids?}"). Text — см. createThreadRequest.
type addMessageRequest struct {
	Text      string   `json:"text"`
	InputType string   `json:"input_type" binding:"required"`
	FileIDs   []string `json:"file_ids"`
}

// addMessageHandler — POST /threads/{id}/messages. Требует SessionAuth.
func addMessageHandler(svc *thread.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req addMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, invalidRequestError("Invalid request body"))
			return
		}
		inputType, err := domain.ParseMessageInputType(req.InputType)
		if err != nil {
			writeError(c, invalidRequestError("Invalid input_type"))
			return
		}

		sess := sessionFromGin(c)
		threadID := c.Param("id")
		t, err := svc.AddMessage(c.Request.Context(), thread.AddMessageRequest{
			ThreadID:  threadID,
			SessionID: sess.ID,
			Language:  sess.Language,
			Text:      req.Text,
			InputType: inputType,
			FileIDs:   req.FileIDs,
		})
		if err != nil {
			writeThreadError(c, err)
			return
		}

		msgs, err := loadMessagesOrFail(c, svc, t.ID, sess.ID)
		if err != nil {
			return
		}
		c.JSON(http.StatusOK, newThreadDetailResponse(t, msgs))
	}
}

// cancelThreadHandler — POST /threads/{id}/cancel. Требует SessionAuth.
func cancelThreadHandler(svc *thread.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFromGin(c)
		t, err := svc.CancelThread(c.Request.Context(), c.Param("id"), sess.ID)
		if err != nil {
			writeThreadError(c, err)
			return
		}
		c.JSON(http.StatusOK, newThreadResponse(t))
	}
}

// deleteThreadHandler — DELETE /threads/{id}: soft delete. Требует SessionAuth.
func deleteThreadHandler(svc *thread.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFromGin(c)
		if err := svc.DeleteThread(c.Request.Context(), c.Param("id"), sess.ID); err != nil {
			writeThreadError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// messageFeedbackRequest — POST /messages/{id}/feedback (zan-backend-tz-v2.md §3.6).
type messageFeedbackRequest struct {
	Value string `json:"value" binding:"required"`
}

// messageFeedbackHandler — POST /messages/{id}/feedback. Требует SessionAuth.
func messageFeedbackHandler(svc *thread.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req messageFeedbackRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, invalidRequestError("Invalid request body"))
			return
		}
		feedback, err := domain.ParseMessageFeedback(req.Value)
		if err != nil {
			writeError(c, invalidRequestError("Invalid value"))
			return
		}

		sess := sessionFromGin(c)
		msg, err := svc.SetMessageFeedback(c.Request.Context(), c.Param("id"), sess.ID, feedback)
		if err != nil {
			writeThreadError(c, err)
			return
		}
		c.JSON(http.StatusOK, newMessageResponse(msg))
	}
}

// loadMessagesOrFail — общий хвост для CreateThread/AddMessage: тред уже
// создан/обновлён, отдельный запрос за полной историей сообщений для
// детального ответа. Пишет ошибку в c и возвращает err!=nil, если что-то
// пошло не так — вызывающий handler в этом случае просто return.
func loadMessagesOrFail(c *gin.Context, svc *thread.Service, threadID, sessionID string) ([]domain.Message, error) {
	_, msgs, err := svc.GetThread(c.Request.Context(), threadID, sessionID)
	if err != nil {
		logger.FromContext(c.Request.Context()).Error("get_thread_messages_failed",
			slog.Group("context", slog.String("error", err.Error())))
		writeError(c, apierror.Internal())
		return nil, err
	}
	return msgs, nil
}

func writeThreadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, thread.ErrThreadNotFound), errors.Is(err, thread.ErrMessageNotFound):
		writeError(c, notFoundError("thread_not_found", "Thread not found"))
	case errors.Is(err, catalog.ErrServiceNotFound):
		writeError(c, notFoundError("service_not_found", "Service not found"))
	case errors.Is(err, file.ErrNotFound):
		writeError(c, notFoundError("file_not_found", "One or more attached files were not found"))
	case errors.Is(err, thread.ErrEmptyText):
		writeError(c, invalidRequestError("Text must not be empty"))
	case errors.Is(err, thread.ErrUnsupportedInputType):
		writeError(c, invalidRequestError("This input_type is not supported yet"))
	case errors.Is(err, thread.ErrInvalidStatusFilter):
		writeError(c, invalidRequestError("Invalid status filter"))
	case errors.Is(err, thread.ErrThreadClosed):
		writeError(c, &apierror.Error{
			Code:       "thread_closed",
			Message:    "Обращение закрыто. Задайте вопрос в новом обращении",
			HTTPStatus: http.StatusConflict,
		})
	case errors.Is(err, thread.ErrThreadNotActive):
		writeError(c, &apierror.Error{
			Code:       "thread_not_active",
			Message:    "Обращение уже завершено. Задайте вопрос в новом обращении",
			HTTPStatus: http.StatusConflict,
		})
	case errors.Is(err, thread.ErrPaymentRequired):
		writeError(c, &apierror.Error{
			Code:       "payment_required",
			Message:    "Для продолжения нужно оплатить обращение",
			HTTPStatus: http.StatusConflict,
		})
	case errors.Is(err, thread.ErrThreadBusy):
		writeError(c, &apierror.Error{
			Code:       "thread_busy",
			Message:    "Обращение уже обрабатывается, попробуйте чуть позже",
			HTTPStatus: http.StatusConflict,
		})
	case errors.Is(err, thread.ErrCannotCancel):
		writeError(c, &apierror.Error{
			Code:       "thread_cannot_cancel",
			Message:    "Обращение нельзя отменить в текущем статусе",
			HTTPStatus: http.StatusConflict,
		})
	case errors.Is(err, billing.ErrInsufficientBalance):
		writeError(c, &apierror.Error{
			Code:       "payment_required",
			Message:    "Для продолжения нужно оплатить обращение",
			HTTPStatus: http.StatusConflict,
		})
	default:
		logger.FromContext(c.Request.Context()).Error("thread_operation_failed",
			slog.Group("context", slog.String("error", err.Error())))
		writeError(c, apierror.Internal())
	}
}
