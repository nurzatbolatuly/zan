import type { Locale } from './locales';
import type { AgentName, RequestStatus, RequestStepStatus } from '../lib/types';
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
    yourQuestion: string;
    pipelineProgress: string;
    /** Общий баннер статуса запроса над списком шагов — отдельно от history/analytics.statusLabel,
     *  т.к. тут нужны более развёрнутые формулировки ("Обрабатывается…", а не просто "Обрабатывается"). */
    statusLabel: Record<RequestStatus, string>;
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
    answerHeading: string;
    documentHeading: string;
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
  pipeline: Record<AgentName, string> & {
    /** Показывается над списком шагов, когда verification нашла замечания и Агент 1 переспрашивает
     *  заново (см. OrchestratorService.runSearchVerificationLoop) — без этого второй раунд визуально
     *  неотличим от зависания на следующем шаге. */
    reprocessingNotice: string;
  };
  /** Текстовая подпись статуса КАЖДОГО шага пайплайна — иконка/цвет в PipelineSteps.tsx сами по
   *  себе недостаточно заметны, нужен явный текст "в процессе"/"не начат"/"ошибка". */
  pipelineStepStatus: Record<RequestStepStatus, string>;
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
