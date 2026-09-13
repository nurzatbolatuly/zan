import { Controller, Get, Inject } from '@nestjs/common';
import { AnalyticsService } from './analytics.service.js';
import type { AnalyticsSummary } from './analytics.types.js';

/**
 * Без аутентификации — как и /metrics, аналитика не привязана к пользователю (агрегация по
 * всем запросам), а полноценных ролей/админ-доступа в проекте пока нет (см. личный кабинет
 * выше в этом же Этапе 13 — там сознательно остались анонимные сессии, не аккаунты).
 * Ограничена глобальным ThrottlerGuard (см. app.module.ts), отдельного лимита не требует —
 * запрос дешёвый (несколько агрегирующих SQL-запросов, без LLM).
 */
@Controller('analytics')
export class AnalyticsController {
  constructor(@Inject(AnalyticsService) private readonly analytics: AnalyticsService) {}

  @Get('summary')
  async getSummary(): Promise<AnalyticsSummary> {
    return this.analytics.getSummary();
  }
}
