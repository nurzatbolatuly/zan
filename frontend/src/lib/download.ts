const EXTENSION_BY_MIME: Record<string, string> = {
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document': 'docx',
};

export function buildDocumentFileName(title: string, fileFormat: string): string {
  const extension = EXTENSION_BY_MIME[fileFormat] ?? 'bin';
  const safeTitle =
    title
      .trim()
      .replace(/[\\/:*?"<>|]+/g, '')
      .slice(0, 100) || 'документ';
  return `${safeTitle}.${extension}`;
}

/** Backend отдаёт файл документа как base64 в JSON (см. lib/types.ts) — декодируем и скачиваем в браузере. */
export function downloadBase64File(
  base64Content: string,
  fileName: string,
  mimeType: string,
): void {
  const byteCharacters = atob(base64Content);
  const byteNumbers = new Array<number>(byteCharacters.length);
  for (let i = 0; i < byteCharacters.length; i += 1) {
    byteNumbers[i] = byteCharacters.charCodeAt(i);
  }
  const blob = new Blob([new Uint8Array(byteNumbers)], { type: mimeType });

  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}
