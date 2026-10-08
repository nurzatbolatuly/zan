import type { DocumentTemplate, DocumentType, TemplateGroup } from "./types";

/** Лимит названия шаблона — тот же, что у бэка (`openapi.yaml#DocumentTemplate.title`). */
export const TEMPLATE_TITLE_MAX_LENGTH = 200;

/** `accept` файлового инпута — те же форматы, что принимает бэк (PDF/DOCX). */
export const TEMPLATE_FILE_ACCEPT = ".pdf,.docx";

const SUPPORTED_EXTENSIONS = [".pdf", ".docx"] as const;

/**
 * Предпроверка формата по расширению — до загрузки, чтобы не гонять файл на
 * сервер ради `415`. Окончательно формат проверяет бэк (по содержимому).
 */
export function isSupportedTemplateFile(fileName: string): boolean {
  const lower = fileName.toLowerCase();
  return SUPPORTED_EXTENSIONS.some((ext) => lower.endsWith(ext));
}

/** Название шаблона по умолчанию — имя файла без расширения. */
export function titleFromFileName(fileName: string): string {
  const dot = fileName.lastIndexOf(".");
  return (dot > 0 ? fileName.slice(0, dot) : fileName).trim();
}

/**
 * Шаблоны, разложенные по типам в порядке справочника (бэк отдаёт его по
 * алфавиту); типы без шаблонов не показываются, порядок шаблонов внутри
 * типа — как пришёл с бэка (новые сверху).
 */
export function groupTemplatesByType(
  templates: DocumentTemplate[],
  types: DocumentType[],
): TemplateGroup[] {
  return types
    .map((type) => ({
      type,
      templates: templates.filter((template) => template.typeId === type.id),
    }))
    .filter((group) => group.templates.length > 0);
}

/** Сколько шаблонов у каждого типа — удалить можно только тип без шаблонов. */
export function countTemplatesByType(templates: DocumentTemplate[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const template of templates) {
    counts.set(template.typeId, (counts.get(template.typeId) ?? 0) + 1);
  }
  return counts;
}
