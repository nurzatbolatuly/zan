package httpserver

import (
	"log/slog"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/gin-gonic/gin"

	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/thread"
	"zan-backend/internal/wshub"
)

// wsThreadEvent — wire-форма событий GET /ws/threads/{id} (Stage 9,
// см. openapi/openapi.yaml, документационная схема WsThreadEvent — OpenAPI
// формально не описывает WS-эндпоинты, схема там существует только как
// письменный источник точных имён полей). answer_done переиспользует
// messageResponse (newMessageResponse) — форма сообщения в WS байт-в-байт
// совпадает с REST, вторая DTO под то же самое не заводится.
type wsThreadEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
	Seq      int    `json:"seq"`

	// thread_status
	Status      string `json:"status,omitempty"`
	PreviewText string `json:"preview_text,omitempty"`

	// answer_delta
	Delta string `json:"delta,omitempty"`

	// answer_done (Status выше тоже заполнен — done)
	Message *messageResponse `json:"message,omitempty"`

	// error — ErrorMessage, не Message: имя нарочно другое, чтобы не
	// коллизировать с полем answer_done.message в общем TS-юнионе на фронте.
	Code         string `json:"code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

func toWireEvent(ev wshub.Event) wsThreadEvent {
	out := wsThreadEvent{Type: string(ev.Type), ThreadID: ev.ThreadID, Seq: ev.Seq}
	switch ev.Type {
	case wshub.EventThreadStatus:
		out.Status = string(ev.Status)
		out.PreviewText = ev.PreviewText
	case wshub.EventAnswerDelta:
		out.Delta = ev.Delta
	case wshub.EventAnswerDone:
		out.Status = string(ev.Status)
		msg := newMessageResponse(ev.Message)
		out.Message = &msg
	case wshub.EventError:
		out.Code = ev.Code
		out.ErrorMessage = ev.ErrorMsg
	}
	return out
}

// threadWebSocketHandler — GET /ws/threads/{id} (Stage 9, WS-стриминг
// статуса и токенов ответа — переоткрытое решение Stage 6, см.
// instructions.md §9). Требует SessionAuth (та же группа маршрутов —
// router.go); extractSessionToken принимает ?token= как фолбэк специально
// ради этого хендшейка — браузерный WebSocket не может выставить
// Authorization-заголовок (session_middleware.go). Владение/существование
// треда проверяется ДО апгрейда обычным HTTP-ответом (writeThreadError,
// 401/404 как у любого REST-эндпоинта) — не разрывом уже установленного
// WS-соединения, с которым нечего сделать так же аккуратно.
func threadWebSocketHandler(hub *wshub.Hub, threadSvc *thread.Service, allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFromGin(c)
		threadID := c.Param("id")

		if _, _, err := threadSvc.GetThread(c.Request.Context(), threadID, sess.ID); err != nil {
			writeThreadError(c, err)
			return
		}

		l := logger.FromContext(c.Request.Context())

		conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
			OriginPatterns: allowedOrigins,
		})
		if err != nil {
			// Accept уже записал свой HTTP-ответ на отклонённый апгрейд
			// (неверный Origin и т.п.) — здесь только лог.
			l.Warn("ws_upgrade_failed", slog.Group("context",
				slog.String("thread_id", threadID),
				slog.String("error", err.Error()),
			))
			return
		}
		defer func() { _ = conn.CloseNow() }()

		// Хендлер держит соединение открытым синхронно (не detached-горутина,
		// в отличие от dispatchProcessing в internal/service/thread) — у
		// каждого HTTP-запроса своя горутина net/http, блокироваться здесь
		// безопасно; c.Request.Context() остаётся валиден весь срок жизни
		// соединения и отменяется ровно тогда, когда этот хендлер вернётся
		// (то есть когда соединение реально закрылось).
		ctx := c.Request.Context()

		sub, snapshot := hub.Subscribe(threadID)
		defer hub.Unsubscribe(threadID, sub)

		for _, ev := range snapshot {
			if err := wsjson.Write(ctx, conn, toWireEvent(ev)); err != nil {
				return
			}
		}

		// Клиент ничего не присылает по протоколу (§1.8) — читающая
		// горутина существует только чтобы заметить закрытие соединения
		// клиентом (Read вернёт ошибку) и разбудить основной цикл ожидания.
		clientClosed := make(chan struct{})
		go func() {
			defer close(clientClosed)
			for {
				if _, _, err := conn.Read(ctx); err != nil {
					return
				}
			}
		}()

		for {
			select {
			case <-clientClosed:
				return
			case ev, ok := <-sub.Events():
				if !ok {
					return
				}
				if err := wsjson.Write(ctx, conn, toWireEvent(ev)); err != nil {
					return
				}
			}
		}
	}
}
