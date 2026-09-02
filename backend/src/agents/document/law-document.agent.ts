import { Inject, Injectable } from '@nestjs/common';
import {
  ALLOWED_LAW_DOMAINS,
  IN_FORCE_LAW_GUARD,
  PROMPT_INJECTION_GUARD,
  delimitDocumentType,
  delimitDraftAnswer,
  delimitUserQuery,
} from '@zan/shared';
import type { ChatClient } from '@zan/shared';
import { CHAT_CLIENT } from '../../llm/llm.constants.js';
import type { DocumentAgent, DocumentAgentInput, DocumentAgentOutput } from '../agent.types.js';
import { buildDocumentBuffer } from './docx-template.js';

export const DOCX_MIME_TYPE =
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document';

const OUT_OF_TOPIC_TITLE = 'OUT_OF_TOPIC';

/**
 * Эвристика, не исчерпывающий юридический список (2026-08-28 — см. INSTRUCTIONS.md §9.7):
 * раньше Агент 4 ВСЕГДА делал живой web_search, чтобы проверить, закреплена ли для типа
 * документа официальная форма — для подавляющего большинства реальных типов (обычное заявление,
 * претензия, уведомление) такой формы в законе обычно нет, и вызов инструмента был чистыми
 * потерями времени/денег без пользы для содержания. Процессуальные документы (иск, жалоба на
 * судебный акт, ходатайство) — те, где ГПК/УПК/КоАП РК чаще всего прямо предписывают
 * форму/реквизиты, для них поиск оставлен. Список приблизительный по формулировке типа документа
 * (свободный текст пользователя) — уточнять по факту (обращения, где документ вышел неверной
 * структуры), а не гадать заранее весь перечень процессуальных документов РК.
 */
const FORM_LIKELY_KEYWORDS = ['иск', 'жалоб', 'апелляц', 'кассац', 'ходатайств'];

function mayHaveOfficialForm(documentType: string): boolean {
  const normalized = documentType.toLowerCase();
  return FORM_LIKELY_KEYWORDS.some((keyword) => normalized.includes(keyword));
}

const COMMON_SYSTEM_PROMPT_TAIL = `Тип документа определяет структуру и тон: например, "заявление" — просительная форма от первого
лица к адресату; "претензия" — требовательная форма с указанием на нарушение и срок ответа;
"жалоба" — форма с указанием на нарушение и требуемое действие от органа.

Правило цитирования норм: указывай номер статьи закона только если он подтверждён — либо есть в
черновом ответе, либо (если у тебя есть инструмент в этом обращении) ты сам нашёл и прочитал его
через инструмент на adilet.zan.kz/zan.gov.kz. Если не уверен — формулируй соответствующее
требование в общих фразах, не выдумывая статьи.

Если вопрос/черновик по существу не о праве Республики Казахстан — не генерируй документ, верни
JSON с title="${OUT_OF_TOPIC_TITLE}", recipientLines=[], bodyParagraphs с одной строкой-пояснением на
русском, что документ не может быть составлен вне сферы права РК.

Ответь СТРОГО валидным JSON, без пояснений вокруг, без markdown-разметки и без \`\`\`-оград, ровно
в этой форме:
{"title":"...","recipientLines":["...","..."],"bodyParagraphs":["...","..."]}

- title — короткое официальное название документа (например, "Заявление о расторжении трудового
  договора").
- recipientLines — строки шапки документа (кому адресован, от кого); используй плейсхолдеры
  вида "[ФИО]", "[должность]", "[адрес]" там, где данные неизвестны из вопроса пользователя.
- bodyParagraphs — пункты основного текста документа БЕЗ номеров (нумерация добавляется отдельно
  шаблонизатором), по существу вопроса, официально-деловым языком без канцелярита.

Не отвечай ничем, кроме этого JSON. Формальные требования к визуальному оформлению документа
(A4, Times New Roman 14, шапка справа сверху, нумерация пунктов) гарантирует шаблонизатор — твоя
задача содержание и, если применимо, обязательная по закону структура/реквизиты.

${IN_FORCE_LAW_GUARD}

${PROMPT_INJECTION_GUARD}`;

const WITH_SEARCH_SYSTEM_PROMPT = `Ты — юридический агент, готовящий СОДЕРЖАНИЕ официального документа по законодательству
Республики Казахстан. У тебя есть инструмент для обращения к веб-страницам. Используй его ТОЛЬКО
для поиска на доменах adilet.zan.kz и zan.gov.kz — единственных источниках, которым ты
доверяешь.

Тебе даны вопрос пользователя, тип документа и черновой юридический ответ (проверен независимым
юристом) — используй его как правовую основу содержания. Дополнительно через инструмент найди на
adilet.zan.kz / zan.gov.kz, установлена ли для указанного типа документа официально закреплённая
форма или обязательные реквизиты/структура (например, для исковых заявлений, жалоб на судебные
акты закон может прямо предписывать, что должно быть указано и в каком порядке). Если нашёл —
построй title, recipientLines и bodyParagraphs так, чтобы структура и обязательные элементы
соответствовали найденному. Если для этого типа документа официальной формы не закреплено (или
не нашёл через инструмент) — используй общепринятую деловую структуру такого документа, ничего не
выдумывая как "обязательное требование закона".

${COMMON_SYSTEM_PROMPT_TAIL}`;

const WITHOUT_SEARCH_SYSTEM_PROMPT = `Ты — юридический агент, готовящий СОДЕРЖАНИЕ официального документа по законодательству
Республики Казахстан.

Тебе даны вопрос пользователя, тип документа и черновой юридический ответ (проверен независимым
юристом) — используй его как правовую основу содержания. У тебя нет инструмента поиска в этом
обращении (для этого типа документа официальная форма законом обычно не закреплена) — используй
общепринятую деловую структуру такого документа, ничего не выдумывая как "обязательное требование
закона".

${COMMON_SYSTEM_PROMPT_TAIL}`;

interface DocumentContent {
  title: string;
  recipientLines: string[];
  bodyParagraphs: string[];
}

function isDocumentContent(value: unknown): value is DocumentContent {
  if (typeof value !== 'object' || value === null) return false;
  const record = value as Record<string, unknown>;
  return (
    typeof record.title === 'string' &&
    Array.isArray(record.recipientLines) &&
    record.recipientLines.every((line) => typeof line === 'string') &&
    Array.isArray(record.bodyParagraphs) &&
    record.bodyParagraphs.length > 0 &&
    record.bodyParagraphs.every((paragraph) => typeof paragraph === 'string')
  );
}

function parseDocumentContent(raw: string): DocumentContent {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    throw new Error(
      'Не удалось разобрать содержимое документа, сгенерированное LLM (невалидный JSON)',
    );
  }
  if (!isDocumentContent(parsed)) {
    throw new Error('Содержимое документа от LLM не соответствует ожидаемой структуре');
  }
  return parsed;
}

function buildUserPrompt(input: DocumentAgentInput): string {
  return `Вопрос пользователя:\n${delimitUserQuery(input.queryText)}\n\nТип документа:\n${delimitDocumentType(input.documentType)}\n\nЧерновой юридический ответ (проверен независимым юристом):\n${delimitDraftAnswer(input.draftAnswer)}`;
}

/**
 * Агент 4. Получает от оркестратора итоговый draftAnswer уже после цикла search↔verification
 * (см. orchestrator.service.ts), как и Агент 3 — не сырой черновик Агента 1 напрямую.
 *
 * Веб-инструмент выдаётся УСЛОВНО (с 2026-08-28, см. mayHaveOfficialForm выше) — только для
 * типов документов, где реально стоит проверять официальную форму/реквизиты; для остальных
 * содержание строится сразу по общепринятой деловой структуре без лишнего живого поиска.
 * Форматирование по формальным требованиям РК (A4, Times New Roman 14, шапка справа сверху,
 * нумерация пунктов) гарантирует шаблонизатор (docx-template.ts), а не текстовая просьба к
 * модели. При невалидном JSON от LLM шаг падает с ошибкой (оркестратор пометит запрос failed),
 * так как частично сгенерированный официальный документ хуже явной ошибки.
 *
 * Defense-in-depth поверх гейта Этапа 13 и проверок Агентов 1–2: если вопрос/черновик всё же не
 * по теме права РК, модель возвращает валидный JSON с title="OUT_OF_TOPIC" (в норме недостижимо
 * — Агенты 1–2 уже остановили бы пайплайн раньше) — run() превращает это в исключение вместо
 * того, чтобы создавать и сохранять бессмысленный "документ" с таким названием.
 */
@Injectable()
export class LawDocumentAgent implements DocumentAgent {
  constructor(@Inject(CHAT_CLIENT) private readonly chat: ChatClient) {}

  async run(input: DocumentAgentInput): Promise<DocumentAgentOutput> {
    const canSearch = mayHaveOfficialForm(input.documentType);
    const response = await this.chat.complete({
      system: canSearch ? WITH_SEARCH_SYSTEM_PROMPT : WITHOUT_SEARCH_SYSTEM_PROMPT,
      user: buildUserPrompt(input),
      reasoningEffort: 'low',
      ...(canSearch ? { webSearch: { allowedDomains: ALLOWED_LAW_DOMAINS } } : {}),
    });

    const content = parseDocumentContent(response);
    if (content.title === OUT_OF_TOPIC_TITLE) {
      throw new Error(
        content.bodyParagraphs[0] ?? 'Документ не может быть составлен вне сферы права РК.',
      );
    }

    const buffer = await buildDocumentBuffer(content);

    return {
      title: content.title,
      fileFormat: DOCX_MIME_TYPE,
      content: buffer.toString('base64'),
    };
  }
}
