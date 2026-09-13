import type { Locale } from './locales';
import type { AgentName, RequestStatus } from '../lib/types';
import { ru } from './dictionaries/ru';
import { kk } from './dictionaries/kk';

export interface Dictionary {
  meta: { title: string; description: string };
  nav: { home: string; history: string; analytics: string; tagline: string };
  footer: { disclaimer: string };
  home: { heading: string; subheading: string };
  form: {
    queryLabel: string;
    queryPlaceholder: string;
    minLengthHint: (min: number) => string;
    includeDocumentLabel: string;
    documentTypeLabel: string;
    documentTypePlaceholder: string;
    submit: string;
    submitting: string;
    genericError: string;
  };
  status: {
    /** Единственный статус, для которого чат-пузырь ассистента показывает готовый лейбл вместо
     *  своего текста (completed/failed/needs_clarification рендерятся отдельными ветками с более
     *  развёрнутыми формулировками, см. RequestStatusView.tsx). */
    cancelledLabel: string;
    /** Заглушка "печатает…" в чат-пузыре ассистента, пока ещё не создана запись ни одного шага
     *  пайплайна (джоба только легла в очередь) или между шагами — см. currentStageLabel в
     *  RequestStatusView.tsx. */
    processingGeneric: string;
    /** Подсказка под композером следующего сообщения (RequestStatusView.tsx), пока последний
     *  запрос в чате ещё не дошёл до paused-статуса — новый вопрос отправлять рано. */
    waitingForReplyHint: string;
    elapsedLabel: (seconds: number) => string;
    stalledWarning: string;
    failedPrefix: string;
    /** Показывается вместо кнопки скачивания, если шаг document не удался (Этап 17, §9.13) —
     *  fail-open: сам ответ пользователю уже completed, но документ не готов (например, агент
     *  решил, что фактов не хватает, см. INSUFFICIENT_CONTEXT_TITLE в law-document.agent.ts). */
    documentErrorPrefix: (reason: string) => string;
    /** Этап 18: догенерация документа к уже готовому ответу — кнопка/форма/статус под ответом,
     *  которая заменила требование пересказывать вопрос заново в композере (см.
     *  RequestStatusView.tsx, backend requestDocument). */
    attachDocumentButton: string;
    attachDocumentTypeLabel: string;
    attachDocumentSubmit: string;
    attachDocumentCancel: string;
    attachDocumentGenerating: string;
    loading: string;
    loadError: string;
    clarificationHeading: string;
    clarificationPlaceholder: string;
    clarificationSubmit: string;
    clarificationSubmitting: string;
    clarificationError: string;
    cancelButton: string;
    cancelling: string;
    cancelError: string;
    cancelConfirm: string;
  };
  pipeline: Record<AgentName, string>;
  document: { download: (title: string) => string; downloadError: string };
  history: {
    heading: string;
    empty: string;
    openLink: string;
    statusLabel: Record<RequestStatus, string>;
    loading: string;
    loadError: string;
    cancelButton: string;
    cancelling: string;
    cancelError: string;
    cancelConfirm: string;
    /** Строка снизу карточки завершённого запроса — принимает уже отформатированное число
     *  секунд (см. formatSeconds в HistoryView.tsx), не raw ms. */
    processedInLabel: (seconds: string) => string;
  };
  analytics: {
    heading: string;
    totalRequestsLabel: string;
    byStatusHeading: string;
    avgProcessingLabel: string;
    avgProcessingUnit: string;
    loading: string;
    loadError: string;
    statusLabel: Record<RequestStatus, string>;
  };
  localeSwitcher: Record<Locale, string>;
}

const dictionaries: Record<Locale, Dictionary> = { ru, kk };

export function getDictionary(locale: Locale): Dictionary {
  return dictionaries[locale];
}
