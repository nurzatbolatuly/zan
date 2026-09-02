'use client';

import { useState, type FormEvent } from 'react';
import { ApiError, submitClarification } from '../lib/api';
import { useDictionary } from '../i18n/DictionaryProvider';

const MAX_ANSWER_LENGTH = 1000;

/**
 * Ровно один раунд уточнения (Этап 13) — после успешной отправки просто уведомляем родителя,
 * чтобы он подхватил новый статус через уже работающий polling (RequestStatusView), а не
 * дублируем его здесь.
 */
export function ClarificationForm({
  requestId,
  question,
  onSubmitted,
}: {
  requestId: string;
  question: string;
  onSubmitted: () => void;
}) {
  const dict = useDictionary();
  const [answer, setAnswer] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    const trimmed = answer.trim();
    if (!trimmed || isSubmitting) return;

    setIsSubmitting(true);
    setError(null);
    try {
      await submitClarification(requestId, trimmed);
      onSubmitted();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : dict.status.clarificationError);
      setIsSubmitting(false);
    }
  }

  return (
    <section className="rounded-lg border border-amber-300 bg-amber-50 px-4 py-3">
      <h2 className="text-sm font-medium text-amber-900">{dict.status.clarificationHeading}</h2>
      <p className="mt-1 text-sm text-amber-800">{question}</p>
      <form onSubmit={(event) => void handleSubmit(event)} className="mt-3 flex flex-col gap-2">
        <textarea
          value={answer}
          onChange={(event) => setAnswer(event.target.value)}
          rows={3}
          maxLength={MAX_ANSWER_LENGTH}
          placeholder={dict.status.clarificationPlaceholder}
          className="w-full rounded-lg border border-amber-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        />
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button
          type="submit"
          disabled={!answer.trim() || isSubmitting}
          className="self-start rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-slate-300"
        >
          {isSubmitting ? dict.status.clarificationSubmitting : dict.status.clarificationSubmit}
        </button>
      </form>
    </section>
  );
}
