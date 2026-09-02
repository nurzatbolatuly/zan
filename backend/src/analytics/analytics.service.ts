import { Inject, Injectable } from '@nestjs/common';
import type { SupabaseClient } from '@supabase/supabase-js';
import { SUPABASE_ADMIN } from '../db/db.constants.js';
import { computeStepsDurationMs } from '../common/step-duration.js';
import type { RequestStatus } from '../requests/request.types.js';
import type { AnalyticsSummary } from './analytics.types.js';

/**
 * Этап 13 (бэклог) — базовая аналитика по обращениям. "Топ законов" и verified/rejected по
 * статьям убраны (см. PLAN.md §2, пересмотрено): без локального индекса Агент 2 больше не
 * возвращает структурированный список статей, только revisedAnswer/concerns — считать по ним
 * "частые законы" нечем.
 * Только чтение — без бизнес-логики, поэтому не заведён отдельный DI-токен/интерфейс
 * репозитория (в отличие от requests/agents) — это оправдано только там, где нужна подмена
 * реализации или юнит-тест без БД.
 *
 * PostgREST не даёт GROUP BY/агрегаты через query-билдер — считаем на стороне Node.
 * Объём requests для аналитики небольшой, для роста стоит вынести в Postgres RPC-функцию.
 */
/** PostgREST по умолчанию режет любой .select() на 1000 строк без ошибки — без постраничного
 *  сбора счётчики по статусам и средняя длительность молча становятся неверными, как только
 *  requests/request_steps превышают эту границу. */
const PAGE_SIZE = 1000;

@Injectable()
export class AnalyticsService {
  constructor(@Inject(SUPABASE_ADMIN) private readonly supabase: SupabaseClient) {}

  async getSummary(): Promise<AnalyticsSummary> {
    const [requestsByStatus, avgProcessingMs] = await Promise.all([
      this.getRequestsByStatus(),
      this.getAvgProcessingMs(),
    ]);

    return { requestsByStatus, avgProcessingMs };
  }

  /** Постранично собирает все строки для страниц query, а не только первые PAGE_SIZE. */
  private async selectAllRows<T>(
    fetchPage: (from: number, to: number) => PromiseLike<{ data: T[] | null; error: unknown }>,
  ): Promise<T[]> {
    const rows: T[] = [];
    let from = 0;
    for (;;) {
      const { data, error } = await fetchPage(from, from + PAGE_SIZE - 1);
      if (error) throw error;
      const page = data ?? [];
      rows.push(...page);
      if (page.length < PAGE_SIZE) break;
      from += PAGE_SIZE;
    }
    return rows;
  }

  private async getRequestsByStatus(): Promise<{ status: RequestStatus; count: number }[]> {
    const rows = await this.selectAllRows<{ status: RequestStatus }>((from, to) =>
      this.supabase
        .from('requests')
        .select('status')
        .range(from, to)
        .returns<{ status: RequestStatus }[]>(),
    );

    const counts = new Map<RequestStatus, number>();
    for (const row of rows) {
      counts.set(row.status, (counts.get(row.status) ?? 0) + 1);
    }
    return [...counts.entries()]
      .map(([status, count]) => ({ status, count }))
      .sort((a, b) => b.count - a.count);
  }

  /**
   * created_at→updated_at запроса включал бы и время ожидания ответа пользователя на уточнение
   * (needs_clarification тоже трогает updated_at, см. orchestrator.service.ts) — это не время
   * работы пайплайна. Вместо этого берём границы по request_steps: от старта первого шага до
   * завершения последнего — так считается только реальная работа агентов.
   */
  private async getAvgProcessingMs(): Promise<number | null> {
    const rows = await this.selectAllRows<{
      id: string;
      request_steps: { started_at: string | null; finished_at: string | null }[];
    }>((from, to) =>
      this.supabase
        .from('requests')
        .select('id, request_steps(started_at, finished_at)')
        .eq('status', 'completed')
        .range(from, to)
        .returns<
          {
            id: string;
            request_steps: { started_at: string | null; finished_at: string | null }[];
          }[]
        >(),
    );

    const durations = rows
      .map((row) => computeStepsDurationMs(row.request_steps))
      .filter((ms): ms is number => ms !== null);
    if (durations.length === 0) return null;
    return durations.reduce((sum, ms) => sum + ms, 0) / durations.length;
  }
}
