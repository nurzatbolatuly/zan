import type { AgentName, RequestStatus, RequestWithDetails } from '../requests/request.types.js';

export const REQUEST_EVENTS = Symbol('REQUEST_EVENTS');

/** Статусы, после которых для запроса больше не будет событий, пока пользователь не предпримет
 *  явное действие (submitClarification/новый запрос) — то же понятие, что isPaused() во
 *  фронтенде (RequestStatusView.tsx), просто определено независимо на своей стороне канала:
 *  RealtimeGateway закрывает WS-подключения после снапшота с одним из этих статусов (см.
 *  realtime.gateway.ts), фронт по этому же принципу решает, когда останавливать поллинг-фолбэк. */
export const PAUSED_STATUSES: readonly RequestStatus[] = [
  'completed',
  'failed',
  'needs_clarification',
  'cancelled',
];

/** Сообщения WS-канала одного запроса (см. realtime.gateway.ts, frontend/src/lib/api.ts). */
export type RequestEventMessage =
  | { type: 'snapshot'; data: RequestWithDetails }
  | { type: 'token'; agentName: AgentName; delta: string };

/**
 * Публикует изменения состояния запроса подписанным WS-клиентам (см. realtime.gateway.ts) —
 * заменяет 2-секундный поллинг фронтенда на push в реальном времени (Этап 14, см. PLAN.md).
 * Оркестратор (orchestrator.service.ts) и RequestsService — единственные вызывающие: оба уже
 * меняют состояние запроса через RequestsRepository, событие идёт сразу следом за изменением.
 */
export interface RequestEventsPublisher {
  /**
   * Пушит текущее полное состояние запроса — тот же shape, что отдаёт GET /requests/:id — всем
   * клиентам, подписанным на этот requestId. Не бросает исключение при сбое публикации
   * (например, запрос уже никому не интересен, WS недоступен) — это side-эффект для UX, а не
   * часть контракта пайплайна, падать из-за него нельзя (см. realtime.gateway.ts).
   */
  publishSnapshot(requestId: string): Promise<void>;
  /**
   * Кусок потокового текста финального ответа (только Агент "answer", см.
   * law-answer.agent.ts) — отдельно от снапшота, чтобы не гонять весь RequestWithDetails на
   * каждый токен генерации.
   */
  publishToken(requestId: string, agentName: AgentName, delta: string): void;
}
