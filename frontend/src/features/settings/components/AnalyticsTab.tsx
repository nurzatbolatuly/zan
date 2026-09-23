import { Button, Card, EmptyState, Skeleton } from "@/shared/ui";
import { useLangStore } from "@/shared/stores/useLangStore";
import { settingsDictionary } from "../locales";
import { useAdminAnalytics } from "../useAdminAnalytics";
import { StatusBarRow } from "./StatusBarRow";

/**
 * Settings → Аналитика (PLAN.md §5 Stage 4c, Stage 6 — реальный
 * `GET /admin/analytics/overview`). Горизонтальные бары, не полноценный
 * чарт — 6 категорий читаются проще так, чем пирогом.
 */
export function AnalyticsTab() {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang];
  const { dictionary, summary, isLoading, isError, retry } = useAdminAnalytics();

  if (isError) {
    return (
      <Card>
        <EmptyState
          title={t.common.loadError}
          action={<Button onClick={retry}>{t.common.retry}</Button>}
        />
      </Card>
    );
  }

  if (isLoading || !summary) {
    return (
      <Card>
        <Skeleton className="mb-5 h-20" />
        <Skeleton className="h-40" />
      </Card>
    );
  }

  const maxCount = Math.max(...summary.byStatus.map((row) => row.count));

  return (
    <Card>
      <h2 className="text-h3 text-ink">{dictionary.title}</h2>
      <p className="mb-5 text-caption text-muted">{dictionary.subtitle}</p>

      <div className="mb-5 grid grid-cols-1 gap-2.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line p-3.5">
          <div className="mb-1.5 text-caption text-muted">{dictionary.metricTotal}</div>
          <div className="text-h2 text-ink">
            {summary.totalRequests.toLocaleString("ru-RU")}
          </div>
        </div>
        <div className="rounded-lg border border-line p-3.5">
          <div className="mb-1.5 text-caption text-muted">{dictionary.metricAvgTime}</div>
          <div className="text-h2 text-ink">
            {summary.avgResponseTimeLabel ?? dictionary.noDataYet}
          </div>
        </div>
        <div className="rounded-lg border border-line p-3.5">
          <div className="mb-1.5 text-caption text-muted">
            {dictionary.metricSatisfaction}
          </div>
          <div className="text-h2 text-ink">
            {summary.satisfactionRateLabel ?? dictionary.noDataYet}
          </div>
        </div>
      </div>

      <div className="mb-2.5 font-mono text-micro font-bold uppercase text-muted">
        {dictionary.byStatusTitle}
      </div>
      <div className="flex flex-col gap-2">
        {summary.byStatus.map((row) => (
          <StatusBarRow
            key={row.status}
            status={row.status}
            label={dictionary.statusLabel[row.status]}
            count={row.count}
            ratio={maxCount > 0 ? row.count / maxCount : 0}
          />
        ))}
      </div>
    </Card>
  );
}
