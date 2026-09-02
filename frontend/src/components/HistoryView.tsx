'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { ApiError, cancelRequest, fetchRequestHistory } from '../lib/api';
import type { RequestSummary } from '../lib/types';
import { useDictionary, useLocale } from '../i18n/DictionaryProvider';

const STATUS_DOT_STYLES: Record<RequestSummary['status'], string> = {
  pending: 'bg-slate-300',
  processing: 'bg-blue-400',
  needs_clarification: 'bg-amber-400',
  completed: 'bg-emerald-400',
  failed: 'bg-red-400',
  cancelled: 'bg-slate-400',
};

/** Только эти статусы ещё можно отменить (см. RequestsRepository.cancelIfActive на бэкенде —
 *  тот же список, отдельно продублирован, чтобы кнопка не мигала на терминальных статусах). */
const CANCELABLE_STATUSES = new Set<RequestSummary['status']>([
  'pending',
  'processing',
  'needs_clarification',
]);

/** Тот же формат (одна десятая секунды), что и avgProcessingMs в AnalyticsView.tsx — единая
 *  подача времени обработки по всему приложению. */
function formatSeconds(ms: number): string {
  return (ms / 1000).toFixed(1);
}

/** Личный кабинет (Этап 13) — история запросов текущей анонимной сессии, без регистрации. */
export function HistoryView() {
  const dict = useDictionary();
  const locale = useLocale();
  const [items, setItems] = useState<RequestSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [cancellingIds, setCancellingIds] = useState<Set<string>>(new Set());
  const [cancelErrors, setCancelErrors] = useState<Record<string, string>>({});

  async function handleCancel(id: string): Promise<void> {
    // Необратимое действие в один клик легко нажать случайно — короткое подтверждение стоит того.
    if (!window.confirm(dict.history.cancelConfirm)) return;
    setCancellingIds((prev) => new Set(prev).add(id));
    setCancelErrors((prev) => {
      const next = { ...prev };
      delete next[id];
      return next;
    });
    try {
      await cancelRequest(id);
      setItems(
        (prev) =>
          prev?.map((item) => (item.id === id ? { ...item, status: 'cancelled' } : item)) ?? prev,
      );
    } catch (err) {
      setCancelErrors((prev) => ({
        ...prev,
        [id]: err instanceof ApiError ? err.message : dict.history.cancelError,
      }));
    } finally {
      setCancellingIds((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  }

  useEffect(() => {
    let cancelled = false;
    fetchRequestHistory()
      .then((data) => {
        if (!cancelled) setItems(data);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : dict.history.loadError);
      });
    return () => {
      cancelled = true;
    };
  }, [dict.history.loadError]);

  if (error) {
    return <p className="text-sm text-red-600">{error}</p>;
  }

  if (!items) {
    return <p className="text-sm text-slate-500">{dict.history.loading}</p>;
  }

  return (
    <section>
      <h1 className="mb-4 text-lg font-semibold text-slate-900">{dict.history.heading}</h1>
      {items.length === 0 ? (
        <p className="text-sm text-slate-500">{dict.history.empty}</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {items.map((item) => (
            <li
              key={item.id}
              className="flex flex-col gap-2 rounded-lg border border-slate-200 bg-white px-4 py-3"
            >
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 text-xs text-slate-500">
                    <span
                      aria-hidden
                      className={`h-2 w-2 rounded-full ${STATUS_DOT_STYLES[item.status]}`}
                    />
                    <span>{dict.history.statusLabel[item.status]}</span>
                    <span>·</span>
                    <span>{new Date(item.createdAt).toLocaleString(locale)}</span>
                  </div>
                  <p className="mt-1 truncate text-sm text-slate-800">{item.queryText}</p>
                  {cancelErrors[item.id] && (
                    <p className="mt-1 text-xs text-red-600">{cancelErrors[item.id]}</p>
                  )}
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  {CANCELABLE_STATUSES.has(item.status) && (
                    <button
                      type="button"
                      onClick={() => void handleCancel(item.id)}
                      disabled={cancellingIds.has(item.id)}
                      className="text-sm font-medium text-red-600 hover:underline disabled:cursor-not-allowed disabled:text-slate-400"
                    >
                      {cancellingIds.has(item.id)
                        ? dict.history.cancelling
                        : dict.history.cancelButton}
                    </button>
                  )}
                  <Link
                    href={`/${locale}/requests/${item.id}`}
                    className="text-sm font-medium text-blue-700 hover:underline"
                  >
                    {dict.history.openLink}
                  </Link>
                </div>
              </div>
              {item.processingMs !== null && (
                <p className="text-xs text-slate-400">
                  {dict.history.processedInLabel(formatSeconds(item.processingMs))}
                </p>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
