'use client';

import { useState, type FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import { ApiError, createRequest } from '../lib/api';
import { useDictionary, useLocale } from '../i18n/DictionaryProvider';

/** Экспортируются — переиспользуются композером следующего сообщения в чате (см.
 *  RequestStatusView.tsx), у него та же валидация текста вопроса, что и у этой формы. */
export const MIN_QUERY_LENGTH = 10;
export const MAX_QUERY_LENGTH = 4000;
const MAX_DOCUMENT_TYPE_LENGTH = 200;

export function RequestForm() {
  const router = useRouter();
  const locale = useLocale();
  const dict = useDictionary();
  const [queryText, setQueryText] = useState('');
  const [includeDocument, setIncludeDocument] = useState(false);
  const [documentType, setDocumentType] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const trimmedLength = queryText.trim().length;
  const isQueryValid = trimmedLength >= MIN_QUERY_LENGTH && queryText.length <= MAX_QUERY_LENGTH;
  const isDocumentTypeValid = !includeDocument || documentType.trim().length > 0;
  const canSubmit = isQueryValid && isDocumentTypeValid && !isSubmitting;

  async function handleSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (!canSubmit) return;

    setIsSubmitting(true);
    setError(null);
    try {
      const response = await createRequest({
        queryText,
        includeDocument,
        documentType: includeDocument ? documentType.trim() : null,
      });
      router.push(`/${locale}/requests/${response.id}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : dict.form.genericError);
      setIsSubmitting(false);
    }
  }

  return (
    <form onSubmit={(event) => void handleSubmit(event)} className="flex flex-col gap-5">
      <div>
        <label htmlFor="queryText" className="mb-1 block text-sm font-medium text-slate-700">
          {dict.form.queryLabel}
        </label>
        <textarea
          id="queryText"
          value={queryText}
          onChange={(event) => setQueryText(event.target.value)}
          rows={6}
          maxLength={MAX_QUERY_LENGTH}
          placeholder={dict.form.queryPlaceholder}
          className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        />
        <div className="mt-1 flex justify-between text-xs text-slate-400">
          <span>
            {trimmedLength < MIN_QUERY_LENGTH ? dict.form.minLengthHint(MIN_QUERY_LENGTH) : ''}
          </span>
          <span>
            {queryText.length}/{MAX_QUERY_LENGTH}
          </span>
        </div>
      </div>

      <div className="flex items-center gap-2">
        <input
          id="includeDocument"
          type="checkbox"
          checked={includeDocument}
          onChange={(event) => setIncludeDocument(event.target.checked)}
          className="h-4 w-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
        />
        <label htmlFor="includeDocument" className="text-sm text-slate-700">
          {dict.form.includeDocumentLabel}
        </label>
      </div>

      {includeDocument && (
        <div>
          <label htmlFor="documentType" className="mb-1 block text-sm font-medium text-slate-700">
            {dict.form.documentTypeLabel}
          </label>
          <input
            id="documentType"
            type="text"
            value={documentType}
            onChange={(event) => setDocumentType(event.target.value)}
            placeholder={dict.form.documentTypePlaceholder}
            maxLength={MAX_DOCUMENT_TYPE_LENGTH}
            className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>
      )}

      {error && <p className="text-sm text-red-600">{error}</p>}

      <button
        type="submit"
        disabled={!canSubmit}
        className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-slate-300"
      >
        {isSubmitting ? dict.form.submitting : dict.form.submit}
      </button>
    </form>
  );
}
