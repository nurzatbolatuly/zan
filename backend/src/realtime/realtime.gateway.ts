import type { IncomingMessage, Server as HttpServer } from 'node:http';
import type { Duplex } from 'node:stream';
import { Inject, Injectable } from '@nestjs/common';
import { WebSocket, WebSocketServer } from 'ws';
import { createLogger, type Logger } from '@zan/shared';
import { config } from '../config.js';
import { readSessionTokenHashFromCookieHeader } from '../common/session.js';
import { REQUESTS_REPOSITORY, type RequestsRepository } from '../requests/requests.repository.js';
import type { RequestWithDetails } from '../requests/request.types.js';
import { PAUSED_STATUSES, type RequestEventMessage } from './request-events.types.js';

const WS_PATH_PATTERN =
  /^\/ws\/requests\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/i;

/**
 * Живой WS-канал состояния одного запроса (Этап 14 — замена 2с-поллинга на push в реальном
 * времени, см. RequestStatusView.tsx на фронте). Реализован на голом `ws`, не через
 * `@nestjs/websockets`/socket.io — единственный WS-путь в приложении, полноценный адаптер ради
 * него избыточен (тот же принцип, что у ручного `app.use(...)` в main.ts вместо скрытых
 * Nest-абстракций).
 *
 * Авторизация — тот же анонимный session-cookie (`zan_session`, см. common/session.ts), что и у
 * REST-эндпоинтов, но upgrade-событие `http.Server` минует Express/`cookie-parser`, поэтому
 * cookie парсится вручную (readSessionTokenHashFromCookieHeader). `Origin` проверяется явно —
 * WS handshake не подчиняется CORS браузера (`app.enableCors` в main.ts на него не действует):
 * без этой проверки произвольный сторонний сайт мог бы открыть соединение от имени залогиненного
 * в браузере пользователя (WS отправляет cookie автоматически, как обычный fetch) и получить
 * снапшот чужого запроса, просто подсмотрев/угадав его id.
 */
@Injectable()
export class RealtimeGateway {
  private readonly logger: Logger = createLogger('realtime-gateway');
  private readonly wss = new WebSocketServer({ noServer: true });
  private readonly subscribers = new Map<string, Set<WebSocket>>();

  constructor(@Inject(REQUESTS_REPOSITORY) private readonly repo: RequestsRepository) {}

  /** Вызывается один раз из main.ts после app.listen() — WS живёт на том же порту/сервере, что
   *  и REST API, отдельный порт под него не заводим. */
  attach(server: HttpServer): void {
    server.on('upgrade', (req, socket, head) => {
      void this.handleUpgrade(req, socket, head);
    });
  }

  private async handleUpgrade(req: IncomingMessage, socket: Duplex, head: Buffer): Promise<void> {
    const pathname = new URL(req.url ?? '', 'http://localhost').pathname;
    const match = WS_PATH_PATTERN.exec(pathname);
    if (!match) {
      socket.destroy();
      return;
    }
    const requestId = match[1]!;

    if (req.headers.origin !== config.frontendOrigin) {
      this.logger.warn(
        { origin: req.headers.origin, requestId },
        'WS-подключение отклонено: origin не совпадает с frontendOrigin',
      );
      socket.destroy();
      return;
    }

    const ownerTokenHash = readSessionTokenHashFromCookieHeader(req.headers.cookie);
    let owned: boolean;
    try {
      owned = await this.repo.isOwnedBy(requestId, ownerTokenHash);
    } catch (error) {
      this.logger.error(
        { error: error instanceof Error ? error.message : String(error), requestId },
        'Не удалось проверить владение запросом для WS-подключения',
      );
      socket.destroy();
      return;
    }
    if (!owned) {
      // Тот же принцип, что у RequestsService.getById: не подтверждаем факт существования
      // чужого запроса, просто закрываем соединение как для несуществующего id.
      socket.destroy();
      return;
    }

    // Снапшот на момент подключения — не только на будущие broadcast(). Без него окно между
    // REST poll() фронтенда (см. RequestStatusView.tsx refreshAndGoLive) и завершением этого
    // upgrade (сетевой RTT + запрос isOwnedBy выше) — реальный зазор, за который пайплайн вполне
    // успевает дойти до paused-статуса (например, resolveEffectiveQueryText — один быстрый
    // LLM-вызов без web_search). publishSnapshot() в этот момент видит hasSubscribers()===false
    // (эта подписка ещё не зарегистрирована) и просто пропускает публикацию — событие теряется
    // безвозвратно, а не откладывается. Сокет при этом остаётся открытым и молчащим: он не
    // закрывается (закрытие только в broadcast() после paused-статуса), поэтому onclose на
    // фронте (единственный триггер поллинг-фолбэка) не срабатывает — экран навсегда застревает
    // на устаревшем статусе.
    const details = await this.repo.findRequestWithDetails(requestId);
    if (!details) {
      socket.destroy();
      return;
    }

    this.wss.handleUpgrade(req, socket, head, (ws) =>
      this.subscribeWithSnapshot(requestId, ws, details),
    );
  }

  private subscribeWithSnapshot(
    requestId: string,
    ws: WebSocket,
    initialSnapshot: RequestWithDetails,
  ): void {
    ws.send(
      JSON.stringify({ type: 'snapshot', data: initialSnapshot } satisfies RequestEventMessage),
    );
    if (PAUSED_STATUSES.includes(initialSnapshot.status)) {
      // Уже в терминальном/приостановленном статусе на момент подключения — дальше для этого
      // requestId broadcast() ничего не пришлёт (см. её же собственную логику закрытия по
      // PAUSED_STATUSES), поэтому не регистрируем сокет вовсе, а закрываем сразу тем же кодом,
      // что и обычное штатное завершение.
      ws.close(1000, 'request paused');
      return;
    }
    this.subscribe(requestId, ws);
  }

  private subscribe(requestId: string, ws: WebSocket): void {
    let sockets = this.subscribers.get(requestId);
    if (!sockets) {
      sockets = new Set();
      this.subscribers.set(requestId, sockets);
    }
    sockets.add(ws);
    ws.on('close', () => this.unsubscribe(requestId, ws));
    ws.on('error', (error) => {
      this.logger.warn({ error: error.message, requestId }, 'Ошибка WS-соединения');
      this.unsubscribe(requestId, ws);
    });
  }

  private unsubscribe(requestId: string, ws: WebSocket): void {
    const sockets = this.subscribers.get(requestId);
    if (!sockets) return;
    sockets.delete(ws);
    if (sockets.size === 0) this.subscribers.delete(requestId);
  }

  /** Позволяет RequestEventsService не тратить SELECT на снапшот, если у запроса в моменте нет
   *  ни одной открытой вкладки браузера (типичный случай — фоновая обработка в очереди). */
  hasSubscribers(requestId: string): boolean {
    return (this.subscribers.get(requestId)?.size ?? 0) > 0;
  }

  /**
   * После снапшота с paused-статусом (см. PAUSED_STATUSES) для этого requestId до следующего
   * явного действия пользователя больше не будет событий — закрываем сокеты сами, а не оставляем
   * фронту гадать, когда переставать слушать (см. RequestStatusView.tsx: реконнект после этого
   * происходит только явно, из resumeLiveUpdates()).
   */
  broadcast(requestId: string, message: RequestEventMessage): void {
    const sockets = this.subscribers.get(requestId);
    if (!sockets || sockets.size === 0) return;

    const payload = JSON.stringify(message);
    for (const ws of sockets) {
      if (ws.readyState === WebSocket.OPEN) ws.send(payload);
    }

    if (message.type === 'snapshot' && PAUSED_STATUSES.includes(message.data.status)) {
      for (const ws of sockets) ws.close(1000, 'request paused');
      this.subscribers.delete(requestId);
    }
  }
}
