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
import { buildDocumentBuffer, type DocumentFormatting } from './docx-template.js';

export const DOCX_MIME_TYPE =
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document';

const OUT_OF_TOPIC_TITLE = 'OUT_OF_TOPIC';
const INSUFFICIENT_CONTEXT_TITLE = 'INSUFFICIENT_CONTEXT';

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

Если фактов из вопроса пользователя и чернового ответа НЕ ХВАТАЕТ, чтобы составить содержательный,
конкретный по этому делу документ запрошенного типа (а не универсальный шаблон ни о чём) — не
выдумывай недостающие факты и не выдавай общие фразы за содержание. Вместо этого верни JSON с
title="${INSUFFICIENT_CONTEXT_TITLE}", recipientLines=[], bodyParagraphs с ОДНОЙ строкой на русском,
конкретно перечисляющей, каких именно фактов не хватает (например: "Не хватает точной даты
увольнения, номера приказа и названия работодателя, чтобы составить заявление по существу"), а не
общей фразой вроде "недостаточно информации".

Отдельно от нехватки фактов для документа В ЦЕЛОМ — если КОНКРЕТНОЕ значение неизвестно, но
остального достаточно, чтобы документ имел смысл (например, известна суть требования, но не
известны ФИО ответчика, дата или сумма) — не выдумывай правдоподобное значение и не пропускай
поле молча. Подставь плейсхолдер в квадратных скобках, описывающий, что туда вписать: "[ФИО]",
"[должность]", "[адрес]", "[дата]", "[номер приказа]", "[сумма]" и т.п. — везде, где конкретное
значение не следует явно из вопроса пользователя или чернового ответа, и в шапке (recipientLines),
и в тексте документа (bodyParagraphs). Пользователь сам впишет то, что не удалось определить, —
это ожидаемая часть готового документа, а не признак того, что документ не готов.

Ответь СТРОГО валидным JSON, без пояснений вокруг, без markdown-разметки и без \`\`\`-оград, ровно
в этой форме:
{"title":"...","recipientLines":["...","..."],"bodyParagraphs":["...","..."],"formatting":{...}}

- title — короткое официальное название документа (например, "Заявление о расторжении трудового
  договора").
- recipientLines — строки шапки документа (кому адресован, от кого); плейсхолдеры — см. правило
  выше.
- bodyParagraphs — пункты основного текста документа БЕЗ номеров (нумерация добавляется отдельно
  шаблонизатором), по существу вопроса, официально-деловым языком без канцелярита.
- formatting — НЕОБЯЗАТЕЛЬНОЕ поле, визуальное оформление документа (шрифт, интервал, отступы,
  поля страницы). Заполняй его полями ТОЛЬКО если в этом обращении у тебя был инструмент поиска и
  ты реально нашёл на adilet.zan.kz/zan.gov.kz норму закона, которая прямо регламентирует
  соответствующий параметр именно для этого типа документа. Ничего не предполагай "по обычаю
  делопроизводства" и не заполняй поле "на всякий случай" — для подавляющего большинства типов
  документов (обычное заявление, претензия и т.п.) закон визуальное оформление вообще не
  регламентирует, и тогда formatting нужно ПОЛНОСТЬЮ ОПУСТИТЬ (не присылать ключ вовсе), а не
  заполнять его "типовыми" значениями наподобие ГОСТа по делопроизводству — так документ получит
  обычное оформление Word по умолчанию, вместо выдуманного сервисом "стандарта". Если инструмента
  поиска в этом обращении у тебя нет — formatting всегда опускай, тебе неоткуда это проверить.
  Доступные поля (все необязательны по отдельности, включай только те, что реально нашёл и можешь
  сослаться на конкретную норму): fontFamily (строка), fontSizePt (число, пункты),
  lineSpacingMultiplier (число — 1, 1.5, 2 и т.п.), firstLineIndentCm (число, см — отступ первой
  строки пунктов текста), pageWidthMm/pageHeightMm (число, мм — только вместе),
  marginTopMm/marginBottomMm/marginLeftMm/marginRightMm (число, мм).

Не отвечай ничем, кроме этого JSON. Нумерацию пунктов и сборку итогового .docx-файла из этих
полей делает шаблонизатор — твоя задача содержание, обязательная по закону структура/реквизиты
и, отдельно, formatting, если для него есть реальное основание в законе (см. выше).

${IN_FORCE_LAW_GUARD}

${PROMPT_INJECTION_GUARD}`;

const WITH_SEARCH_SYSTEM_PROMPT = `Ты — юридический агент, готовящий СОДЕРЖАНИЕ официального документа по законодательству
Республики Казахстан. У тебя есть инструмент для обращения к веб-страницам. Используй его ТОЛЬКО
для поиска на доменах adilet.zan.kz и zan.gov.kz — единственных источниках, которым ты
доверяешь.

Тебе даны вопрос пользователя, тип документа и черновой юридический ответ — используй его как
правовую основу содержания. Дополнительно через инструмент найди на
adilet.zan.kz / zan.gov.kz, установлена ли для указанного типа документа официально закреплённая
форма или обязательные реквизиты/структура (например, для исковых заявлений, жалоб на судебные
акты закон может прямо предписывать, что должно быть указано и в каком порядке). Если нашёл —
построй title, recipientLines и bodyParagraphs так, чтобы структура и обязательные элементы
соответствовали найденному. Если для этого типа документа официальной формы не закреплено (или
не нашёл через инструмент) — используй общепринятую деловую структуру такого документа, ничего не
выдумывая как "обязательное требование закона".

Заодно, тем же поиском, проверь, регламентирует ли закон визуальное оформление (шрифт, отступы,
межстрочный интервал, поля страницы) именно для этого типа документа — см. правило про поле
formatting ниже. Процессуальные нормы РК чаще определяют только структуру/реквизиты, а не
типографику, так что для большинства типов документов ответ будет "не регламентирует" — это
нормальный исход, тогда formatting просто не заполняется.

${COMMON_SYSTEM_PROMPT_TAIL}`;

const WITHOUT_SEARCH_SYSTEM_PROMPT = `Ты — юридический агент, готовящий СОДЕРЖАНИЕ официального документа по законодательству
Республики Казахстан.

Тебе даны вопрос пользователя, тип документа и черновой юридический ответ — используй его как
правовую основу содержания. У тебя нет инструмента поиска в этом
обращении (для этого типа документа официальная форма законом обычно не закреплена) — используй
общепринятую деловую структуру такого документа, ничего не выдумывая как "обязательное требование
закона". Раз инструмента поиска нет, ты не можешь проверить, регламентирует ли закон визуальное
оформление документа, — поэтому поле formatting в ответе всегда полностью опускай (см. правило
про него ниже).

${COMMON_SYSTEM_PROMPT_TAIL}`;

interface DocumentContent {
  title: string;
  recipientLines: string[];
  bodyParagraphs: string[];
  /** См. DocumentFormatting и правило про это поле в COMMON_SYSTEM_PROMPT_TAIL выше — заполнено,
   *  только если модель реально нашла соответствующую норму закона через web_search. */
  formatting?: DocumentFormatting;
}

const FORMATTING_STRING_FIELDS = ['fontFamily'] as const;
const FORMATTING_NUMBER_FIELDS = [
  'fontSizePt',
  'lineSpacingMultiplier',
  'firstLineIndentCm',
  'pageWidthMm',
  'pageHeightMm',
  'marginTopMm',
  'marginBottomMm',
  'marginLeftMm',
  'marginRightMm',
] as const;

/**
 * Строже, чем просто "это объект": каждое присутствующее поле должно быть заявленного типа —
 * модель иногда путает типы (например, отдаёт "14" строкой вместо числа 14), а мусор здесь без
 * проверки ушёл бы прямо в docx-template.ts (см. convertMillimetersToTwip/Math.round там),
 * потенциально уронив генерацию документа на ровном месте вместо явной ошибки здесь.
 */
function isValidFormatting(value: unknown): value is DocumentFormatting {
  if (typeof value !== 'object' || value === null) return false;
  const record = value as Record<string, unknown>;
  return (
    FORMATTING_STRING_FIELDS.every(
      (field) => record[field] === undefined || typeof record[field] === 'string',
    ) &&
    FORMATTING_NUMBER_FIELDS.every(
      (field) => record[field] === undefined || typeof record[field] === 'number',
    )
  );
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
    record.bodyParagraphs.every((paragraph) => typeof paragraph === 'string') &&
    (record.formatting === undefined || isValidFormatting(record.formatting))
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
  return `Вопрос пользователя:\n${delimitUserQuery(input.queryText)}\n\nТип документа:\n${delimitDocumentType(input.documentType)}\n\nЧерновой юридический ответ:\n${delimitDraftAnswer(input.draftAnswer)}`;
}

/**
 * Агент "document". Получает от оркестратора итоговый ответ Агента "answer" (см.
 * orchestrator.service.ts, agent.types.ts) — единственного содержательного шага пайплайна после
 * слияния search→verification→editor в один промпт (см. INSTRUCTIONS.md §9.10).
 *
 * Веб-инструмент выдаётся УСЛОВНО (с 2026-08-28, см. mayHaveOfficialForm выше) — только для
 * типов документов, где реально стоит проверять официальную форму/реквизиты; для остальных
 * содержание строится сразу по общепринятой деловой структуре без лишнего живого поиска. Когда
 * поиск нужен — продолжает Responses API диалог Агента "answer" (`previousResponseId`, см.
 * chatClient.ts) вместо нового с нуля (см. §9.8 INSTRUCTIONS.md).
 * Визуальное оформление (шрифт, интервал, отступы, поля страницы, шапка справа сверху, нумерация
 * пунктов) шаблонизатор (docx-template.ts) сам не решает вовсе — никаких зашитых в код "типовых"
 * значений (раньше здесь были захардкожены A4/Times New Roman 14/красная строка/полуторный
 * интервал по аналогии с общим делопроизводством, но это никогда не проверялось на соответствие
 * закону РК для конкретных типов документов сервиса). Источник этих значений — только модель,
 * через поле `formatting` в JSON-ответе (см. isValidFormatting/COMMON_SYSTEM_PROMPT_TAIL): она
 * заполняет его, только если реально нашла норму закона через web_search в этом обращении, иначе
 * оставляет пустым — тогда шаблонизатор просто не трогает соответствующий аспект оформления, и
 * применяется обычный дефолт docx/Word. При невалидном JSON от LLM шаг падает с ошибкой
 * (оркестратор пометит запрос failed), так как частично сгенерированный официальный документ хуже
 * явной ошибки.
 *
 * Defense-in-depth поверх гейта Этапа 13 и проверки Агента "answer": если вопрос/черновик всё же
 * не по теме права РК, модель возвращает валидный JSON с title="OUT_OF_TOPIC" (в норме
 * недостижимо — Агент "answer" уже остановил бы пайплайн раньше) — run() превращает это в
 * исключение вместо того, чтобы создавать и сохранять бессмысленный "документ" с таким названием.
 *
 * Этап 17 (§9.13): второй сентинел, title="INSUFFICIENT_CONTEXT" — модель осознанно отказывается
 * составлять документ, если фактов из вопроса/чернового ответа не хватает на что-то содержательное
 * (не выдумывает и не выдаёт универсальный шаблон за результат). Это ДОСТИЖИМЫЙ, ожидаемый исход
 * (не defense-in-depth) — оркестратор ловит его как fail-open: пользователь получает текстовый
 * ответ как обычно, а причина отказа от документа — в request_steps этого шага. Отдельно: любое
 * КОНКРЕТНОЕ значение, которого не хватает (ФИО, дата, сумма), но не мешающее документу в целом
 * иметь смысл, — не сентинел, а плейсхолдер вида "[ФИО]" прямо в тексте (см. промпт выше).
 */
@Injectable()
export class LawDocumentAgent implements DocumentAgent {
  constructor(@Inject(CHAT_CLIENT) private readonly chat: ChatClient) {}

  async run(input: DocumentAgentInput): Promise<DocumentAgentOutput> {
    const canSearch = mayHaveOfficialForm(input.documentType);
    const response = canSearch
      ? (
          await this.chat.completeWithWebSearchMeta(
            {
              system: WITH_SEARCH_SYSTEM_PROMPT,
              user: buildUserPrompt(input),
              reasoningEffort: 'low',
              previousResponseId: input.previousResponseId,
              requestId: input.requestId,
            },
            { allowedDomains: ALLOWED_LAW_DOMAINS },
          )
        ).content
      : await this.chat.complete({
          system: WITHOUT_SEARCH_SYSTEM_PROMPT,
          user: buildUserPrompt(input),
          reasoningEffort: 'low',
          requestId: input.requestId,
        });

    const content = parseDocumentContent(response);
    if (content.title === OUT_OF_TOPIC_TITLE) {
      throw new Error(
        content.bodyParagraphs[0] ?? 'Документ не может быть составлен вне сферы права РК.',
      );
    }
    if (content.title === INSUFFICIENT_CONTEXT_TITLE) {
      // Оркестратор ловит это исключение как fail-open (см. runDocumentStep в
      // orchestrator.service.ts) — не роняет весь ответ пользователю, только сам документ.
      throw new Error(
        content.bodyParagraphs[0] ?? 'Недостаточно данных, чтобы составить документ по существу.',
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
