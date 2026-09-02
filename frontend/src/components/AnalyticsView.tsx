'use client';

import { useEffect, useState } from 'react';
import { ApiError, fetchAnalytics } from '../lib/api';
import type { AnalyticsSummary, RequestStatus } from '../lib/types';
import { useDictionary } from '../i18n/DictionaryProvider';

/**
 * Цвета — из статусной палитры (references/palette.md скилла dataviz, фиксированная, не
 * тематизируется): good/warning/critical. 'pending'/'processing' — не статусы успеха/провала,
 * а нейтральные "в очереди"/"в работе", поэтому вне фиксированной 4-слотовой палитры —
 * приглушённый серый и последовательный синий соответственно. Статусный жёлтый
 * (#fab219) даёт контраст < 3:1 на светлой поверхности "by design" (см. палитру) — поэтому
 * везде рядом с цветом обязательна видимая текстовая подпись, а не только цвет.
 */
const STATUS_COLOR: Record<RequestStatus, string> = {
  pending: '#898781',
  processing: '#2a78d6',
  needs_clarification: '#fab219',
  completed: '#0ca30c',
  failed: '#d03b3b',
  cancelled: '#898781',
};

const STATUS_ORDER: RequestStatus[] = [
  'pending',
  'processing',
  'needs_clarification',
  'completed',
  'failed',
  'cancelled',
];

function formatMs(ms: number): string {
  return (ms / 1000).toFixed(1);
}

function RankedBar({
  label,
  value,
  max,
  color,
}: {
  label: string;
  value: number;
  max: number;
  color: string;
}) {
  const widthPercent = max > 0 ? Math.max((value / max) * 100, value > 0 ? 4 : 0) : 0;
  return (
    <div className="flex items-center gap-3 text-sm">
      <span className="w-40 shrink-0 truncate text-slate-700" title={label}>
        {label}
      </span>
      <span className="h-2 flex-1 rounded-full bg-slate-100">
        <span
          className="block h-2 rounded-full"
          style={{ width: `${widthPercent}%`, backgroundColor: color }}
        />
      </span>
      <span className="w-8 shrink-0 text-right text-xs font-medium text-slate-600">{value}</span>
    </div>
  );
}

export function AnalyticsView() {
  const dict = useDictionary();
  const [summary, setSummary] = useState<AnalyticsSummary | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetchAnalytics()
      .then((data) => {
        if (!cancelled) setSummary(data);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : dict.analytics.loadError);
      });
    return () => {
      cancelled = true;
    };
  }, [dict.analytics.loadError]);

  if (error) {
    return <p className="text-sm text-red-600">{error}</p>;
  }

  if (!summary) {
    return <p className="text-sm text-slate-500">{dict.analytics.loading}</p>;
  }

  const countByStatus = new Map(summary.requestsByStatus.map((row) => [row.status, row.count]));
  const totalRequests = summary.requestsByStatus.reduce((sum, row) => sum + row.count, 0);
  const maxStatusCount = Math.max(1, ...summary.requestsByStatus.map((row) => row.count));

  return (
    <section className="flex flex-col gap-8">
      <h1 className="text-lg font-semibold text-slate-900">{dict.analytics.heading}</h1>

      <div className="grid grid-cols-2 gap-3">
        <div className="rounded-lg border border-slate-200 bg-white px-4 py-3">
          <p className="text-xs text-slate-500">{dict.analytics.totalRequestsLabel}</p>
          <p className="mt-1 text-2xl font-semibold text-slate-900">{totalRequests}</p>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white px-4 py-3">
          <p className="text-xs text-slate-500">{dict.analytics.avgProcessingLabel}</p>
          <p className="mt-1 text-2xl font-semibold text-slate-900">
            {summary.avgProcessingMs === null ? '—' : formatMs(summary.avgProcessingMs)}
            {summary.avgProcessingMs !== null && (
              <span className="ml-1 text-sm font-normal text-slate-500">
                {dict.analytics.avgProcessingUnit}
              </span>
            )}
          </p>
        </div>
      </div>

      <div>
        <h2 className="mb-3 text-sm font-medium text-slate-700">
          {dict.analytics.byStatusHeading}
        </h2>
        <div className="flex flex-col gap-2 rounded-lg border border-slate-200 bg-white px-4 py-3">
          {STATUS_ORDER.map((status) => (
            <RankedBar
              key={status}
              label={dict.analytics.statusLabel[status]}
              value={countByStatus.get(status) ?? 0}
              max={maxStatusCount}
              color={STATUS_COLOR[status]}
            />
          ))}
        </div>
      </div>
    </section>
  );
}
