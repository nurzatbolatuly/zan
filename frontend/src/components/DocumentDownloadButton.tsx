'use client';

import { useState } from 'react';
import { buildDocumentFileName, downloadBase64File } from '../lib/download';
import type { DocumentRecord } from '../lib/types';
import { useDictionary } from '../i18n/DictionaryProvider';

export function DocumentDownloadButton({ document }: { document: DocumentRecord }) {
  const dict = useDictionary();
  const [error, setError] = useState<string | null>(null);

  function handleClick(): void {
    try {
      downloadBase64File(
        document.content,
        buildDocumentFileName(document.title, document.fileFormat),
        document.fileFormat,
      );
      setError(null);
    } catch {
      // atob() бросает на битом/усечённом base64 от бэкенда — без этого кнопка молча ничего не
      // делает и пользователь не понимает, что скачивание не удалось.
      setError(dict.document.downloadError);
    }
  }

  return (
    <div>
      <button
        type="button"
        onClick={handleClick}
        className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-100"
      >
        {dict.document.download(document.title)}
      </button>
      {error && <p className="mt-1 text-xs text-red-600">{error}</p>}
    </div>
  );
}
