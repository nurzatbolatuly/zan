import { Card } from "@/shared/ui";
import { useLangStore } from "@/shared/stores/useLangStore";
import { settingsDictionary } from "../locales";
import { ANALYTICS_MOCKS } from "../mocks";
import { StatusBarRow } from "./StatusBarRow";

/**
 * Settings → Аналитика (PLAN.md §5 Stage 4c). Горизонтальные бары, не
 * полноценный чарт — 6 категорий читаются проще так, чем пирогом; навык
 * `dataviz` не требуется на этом этапе (см. PLAN.md §5 Stage 4c, дословно).
 */
export function AnalyticsTab() {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang].analytics;
  const analytics = ANALYTICS_MOCKS[lang];
  const maxCount = Math.max(...analytics.byStatus.map((row) => row.count));

  return (
    <Card>
      <h2 className="text-h3 text-ink">{t.title}</h2>
      <p className="mb-5 text-caption text-muted">{t.subtitle}</p>

      <div className="mb-5 grid grid-cols-1 gap-2.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line p-3.5">
          <div className="mb-1.5 text-caption text-muted">{t.metricTotal}</div>
          <div className="text-h2 text-ink">
            {analytics.totalRequests.toLocaleString("ru-RU")}
          </div>
        </div>
        <div className="rounded-lg border border-line p-3.5">
          <div className="mb-1.5 text-caption text-muted">{t.metricAvgTime}</div>
          <div className="text-h2 text-ink">{analytics.avgResponseTimeLabel}</div>
        </div>
        <div className="rounded-lg border border-line p-3.5">
          <div className="mb-1.5 text-caption text-muted">{t.metricSatisfaction}</div>
          <div className="text-h2 text-ink">{analytics.satisfactionRateLabel}</div>
        </div>
      </div>

      <div className="mb-2.5 font-mono text-micro font-bold uppercase text-muted">
        {t.byStatusTitle}
      </div>
      <div className="flex flex-col gap-2">
        {analytics.byStatus.map((row) => (
          <StatusBarRow
            key={row.status}
            status={row.status}
            label={t.statusLabel[row.status]}
            count={row.count}
            ratio={maxCount > 0 ? row.count / maxCount : 0}
          />
        ))}
      </div>
    </Card>
  );
}
