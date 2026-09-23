import { getWsBaseUrl } from "./env";
import { logger } from "./logger";
import { useSessionStore } from "@/shared/stores/useSessionStore";
import type { ThreadStatus } from "@/shared/types/common";
import type { MessageDto } from "@/shared/types/api";

/**
 * Wire-протокол `GET /ws/threads/{id}` (см. instructions.md, «Транспорт
 * сессии» и раздел про WS-события в openapi.yaml) — один JSON-объект на
 * server-push сообщение, различаются по `type`. Сервер только пишет, клиент
 * ничего не отправляет (кроме имплицитного закрытия соединения).
 */
export interface ThreadStatusEvent {
  type: "thread_status";
  thread_id: string;
  seq: number;
  status: ThreadStatus;
  preview_text: string;
}

export interface AnswerDeltaEvent {
  type: "answer_delta";
  thread_id: string;
  seq: number;
  delta: string;
}

export interface AnswerDoneEvent {
  type: "answer_done";
  thread_id: string;
  seq: number;
  status: ThreadStatus;
  message: MessageDto;
}

export interface ThreadWsErrorEvent {
  type: "error";
  thread_id: string;
  seq: number;
  code: string;
  // Не "message" — коллизия с AnswerDoneEvent.message: MessageDto в этом же юнионе.
  error_message: string;
}

export type ThreadSocketEvent =
  ThreadStatusEvent | AnswerDeltaEvent | AnswerDoneEvent | ThreadWsErrorEvent;

export interface ThreadSocketHandlers {
  onEvent: (event: ThreadSocketEvent) => void;
  onOpen?: () => void;
  onClose?: () => void;
}

export interface ThreadSocketHandle {
  close: () => void;
}

const RECONNECT_BASE_DELAY_MS = 500;
const RECONNECT_MAX_DELAY_MS = 5000;
const RECONNECT_MAX_ATTEMPTS = 5;

/**
 * Один WS-канал на активно открытый тред (не один сокет на сессию) —
 * `GET /ws/threads/{id}?token=<session_token>`. Токен — query-параметр, не
 * заголовок: нативный браузерный WebSocket не умеет выставлять
 * `Authorization` на хендшейке, это единственное отступление от общего
 * bearer-механизма `shared/lib/api.ts` (см. instructions.md, «Транспорт
 * сессии») — только для WS-апгрейда, REST по-прежнему ходит заголовком.
 *
 * Реконнект — экспоненциальный backoff с потолком, ограниченное число
 * попыток. Безопасно по построению: хаб на бэке при каждом (пере)подключении
 * отдаёт снапшот (текущий статус + накопленный текст дельт текущего раунда),
 * клиенту не нужно помнить, на чём он остановился.
 */
export function openThreadSocket(
  threadId: string,
  handlers: ThreadSocketHandlers,
): ThreadSocketHandle {
  let socket: WebSocket | null = null;
  let closedByCaller = false;
  let attempt = 0;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  function connect(): void {
    const token = useSessionStore.getState().sessionToken;
    const query = token ? `?token=${encodeURIComponent(token)}` : "";
    const url = `${getWsBaseUrl()}/ws/threads/${threadId}${query}`;

    const ws = new WebSocket(url);
    socket = ws;

    ws.onopen = () => {
      attempt = 0;
      handlers.onOpen?.();
    };

    ws.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data as string) as ThreadSocketEvent;
        handlers.onEvent(parsed);
      } catch (error) {
        logger.error({
          scope: "chat.ws",
          event: "event_parse_failed",
          data: { threadId },
          error,
        });
      }
    };

    ws.onclose = () => {
      handlers.onClose?.();
      if (closedByCaller) return;
      if (attempt >= RECONNECT_MAX_ATTEMPTS) {
        logger.warn({
          scope: "chat.ws",
          event: "reconnect_exhausted",
          data: { threadId, attempts: attempt },
        });
        return;
      }
      const delay = Math.min(
        RECONNECT_BASE_DELAY_MS * 2 ** attempt,
        RECONNECT_MAX_DELAY_MS,
      );
      attempt += 1;
      reconnectTimer = setTimeout(connect, delay);
    };

    // onclose всегда следует за onerror в WebSocket API — реконнект решается
    // там, здесь достаточно не оставлять необработанное событие.
    ws.onerror = () => {};
  }

  connect();

  return {
    close: () => {
      closedByCaller = true;
      if (reconnectTimer !== null) clearTimeout(reconnectTimer);
      socket?.close();
    },
  };
}
