import { useQuery } from "@tanstack/react-query";
import { useLangStore } from "@/shared/stores/useLangStore";
import { api } from "@/shared/lib/api";
import { formatDuration } from "@/shared/lib/format";
import { settingsDictionary } from "./locales";
import type { AnalyticsSummary } from "./types";
import type { AnalyticsOverviewDto } from "@/shared/types/api";
import type { ThreadStatus } from "@/shared/types/common";

/** Порядок строк "по статусам" — 1:1 с исходным демо-порядком (не порядок ключей JSON). */
const STATUS_ORDER: ThreadStatus[] = [
  "done",
  "processing",
  "awaiting_payment",
  "error",
  "canceled",
];

/**
 * Settings → Аналитика (Stage 6 — `GET /admin/analytics/overview`, требует
 * `X-Admin-Token`). `avg_processing_time_sec`/`satisfaction_rate` реально
 * отсутствуют в JSON (`omitempty`), пока не накопилось данных — не `0`,
 * маппится в `null`, не в отформатированный "0 сек"/"0%".
 */
export function useAdminAnalytics() {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang].analytics;

  const query = useQuery({
    queryKey: ["admin", "analytics"],
    queryFn: () =>
      api.get<AnalyticsOverviewDto>("/admin/analytics/overview", { admin: true }),
  });

  const summary: AnalyticsSummary | null = query.data
    ? {
        totalRequests: query.data.total_threads,
        avgResponseTimeLabel:
          query.data.avg_processing_time_sec !== undefined
            ? // formatDuration ожидает целые секунды (см. shared/lib/format.ts) —
              // среднее с бэка может быть дробным.
              formatDuration(Math.round(query.data.avg_processing_time_sec), lang)
            : null,
        satisfactionRateLabel:
          query.data.satisfaction_rate !== undefined
            ? `${Math.round(query.data.satisfaction_rate * 100)}%`
            : null,
        byStatus: STATUS_ORDER.map((status) => ({
          status,
          count: query.data.status_breakdown[status] ?? 0,
        })),
      }
    : null;

  return {
    dictionary: t,
    summary,
    isLoading: query.isLoading,
    isError: query.isError,
    retry: () => void query.refetch(),
  };
}
