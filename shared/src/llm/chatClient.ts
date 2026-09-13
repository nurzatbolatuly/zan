import OpenAI from 'openai';
import { createLogger } from '../logging/createLogger.js';

const logger = createLogger('chat-client');

/**
 * Общий клиент для текстовых LLM-вызовов OpenAI — общий для агентов "answer" и "document"
 * (backend/src/agents/), поэтому вынесен сюда, а не скопирован по одному разу на агента.
 */
/**
 * Замер вживую (2026-08-22, реальные вызовы API): gpt-5.6-terra — 39-102с на один вызов даже
 * при reasoning_effort='low', физически не укладывается в бюджет ответа (агенты вызываются
 * последовательно). gpt-5.6-luna при 'low' — 15-24с на вызов, с полным сохранением глубины
 * ответа (проверено на реальном юр. кейсе — качество определяет промпт, не размер модели).
 *
 * 2026-08-23 переключили на gpt-5.6-sol "по прямому указанию", без замера latency/стоимости —
 * 2026-08-28 вернули обратно на gpt-5.6-luna: единственная модель из трёх с реальным замером,
 * подтверждающим, что качество не проседает на её размере, при заметно лучшей latency/стоимости,
 * чем у terra. Если понадобится другая модель — сначала мерить, потом менять, не наоборот.
 */
export const DEFAULT_CHAT_MODEL = 'gpt-5.6-luna';

/**
 * Модели семейства GPT-5 (в т.ч. gpt-5.6-*) не поддерживают кастомный `temperature` через
 * Chat Completions API — принимают только значение по умолчанию, на любое другое отвечают
 * 400. Поэтому параметр сюда сознательно не вынесен в интерфейс запроса — раньше был, но
 * терял смысл при переходе на эту модель (см. PLAN.md).
 */
export type ReasoningEffort = 'low' | 'medium' | 'high';

export interface ChatCompletionRequest {
  system: string;
  user: string;
  model?: string;
  /**
   * Глубина рассуждения модели (GPT-5.6: none/low/medium/high/xhigh/max, по умолчанию у OpenAI
   * — medium). Не указывать — оставить дефолт (medium) для шагов, где важна глубина
   * рассуждения (синтез ответа, финальная формулировка для пользователя). 'low' — для
   * best-effort/вспомогательных шагов (перепроверка, структурирование уже готового текста),
   * где качество не зависит от глубины рассуждения, а задержка складывается в общее время
   * ответа (агенты вызываются последовательно).
   */
  reasoningEffort?: ReasoningEffort;
  /**
   * Даёт модели встроенный веб-поиск (OpenAI Responses API `web_search`), ограниченный
   * `allowedDomains` — модель физически не может открыть страницу за пределами списка, это
   * enforced на стороне OpenAI, а не только текстовой инструкцией в промпте. Используют Агенты
   * "answer" и (условно) "document" (см. law-answer.agent.ts, law-document.agent.ts) — живьём
   * читают adilet.zan.kz/zan.gov.kz на каждый свой вызов, локального индекса нет (см.
   * INSTRUCTIONS.md §9 — решение вернуться к live-поиску после отказа от retrieval-корпуса).
   * Присутствие этого поля переключает вызов на Responses API вместо Chat Completions — у
   * обычных вызовов без него поведение не меняется.
   */
  webSearch?: { allowedDomains: string[] };
  /**
   * Продолжает существующий Responses API диалог (OpenAI `previous_response_id`) вместо нового —
   * модель видит уже сделанные в этом диалоге tool-вызовы (открытые страницы) и не обязана искать
   * их заново. Используется только вместе с `webSearch` (см. completeWithWebSearchMeta) — цепочка
   * Агент 1 → Агент 2 → (опционально) Агент 4, чтобы независимая перепроверка/подготовка документа
   * точечно открывала уже найденные страницы вместо полного повторного поиска (самый дорогой по
   * latency шаг пайплайна — см. чейнинг в orchestrator.service.ts).
   */
  previousResponseId?: string;
  /**
   * Не отправляется в OpenAI — только для наблюдаемости (см. LlmCallInfo). Позволяет отделить
   * реальное время ответа OpenAI на КОНКРЕТНЫЙ запрос пользователя от общего времени обработки
   * (БД, очередь, накладные расходы оркестратора) — обе величины близки на практике (шаг почти
   * целиком состоит из этого вызова), но только эта измеряется независимо и по ней имеет смысл
   * сверяться при любой будущей оптимизации, а не по суммарной длительности шага.
   */
  requestId?: string;
}

/**
 * Результат вызова через Responses API с `web_search`. `responseId` передаётся следующим вызовом
 * как `previousResponseId`, чтобы продолжить тот же диалог без повторного поиска; `citedUrls` —
 * реальные адреса, которые модель открыла в этом вызове (аннотации `url_citation` в
 * `response.output`) — следующий агент в цепочке проверяет именно их, точечно, а не ищет с нуля.
 */
export interface WebSearchResult {
  content: string;
  responseId: string;
  citedUrls: string[];
}

export interface ChatClient {
  complete(request: ChatCompletionRequest): Promise<string>;
  /**
   * Как `complete()` с `webSearch`, но возвращает метаданные диалога (`responseId`, `citedUrls`)
   * вместо голого текста — нужно агентам, которые дальше передают эту цепочку следующему агенту
   * (см. WebSearchResult). `webSearch` вынесен отдельным параметром, а не полем request, чтобы вызов
   * этого метода без него был невозможен на уровне типов.
   */
  completeWithWebSearchMeta(
    request: Omit<ChatCompletionRequest, 'webSearch'>,
    webSearch: { allowedDomains: string[] },
  ): Promise<WebSearchResult>;
  /**
   * Как `completeWithWebSearchMeta()`, но потоково: `onDelta` вызывается на каждый кусок текста
   * ответа по мере генерации (события `response.output_text.delta` Responses API) — модель
   * пишет текст уже после того, как отработали её собственные вызовы `web_search` внутри этого же
   * обращения, поэтому деltы приходят не с первой секунды, а как только модель начинает
   * формулировать ответ. Используется Агентом "answer" (law-answer.agent.ts) — тем, что
   * пользователь видит по мере генерации, и есть финальный ответ (см. realtime/ в backend).
   */
  completeWithWebSearchMetaStream(
    request: Omit<ChatCompletionRequest, 'webSearch'>,
    webSearch: { allowedDomains: string[] },
    onDelta: (delta: string) => void,
  ): Promise<WebSearchResult>;
}

/**
 * Один сырой вызов OpenAI — снаружи ChatClient (агенты, оркестратор) виден только итоговый
 * текст ответа, поэтому именно здесь, у самой границы с сетевым вызовом, единственное место,
 * где можно замерить его реальную задержку. `operation` различает Chat Completions
 * ('chat') и Responses API с веб-поиском ('responses', см. completeWithWebSearchMeta) — это разные
 * эндпоинты OpenAI с разным профилем задержки, схлопывать их в одну метрику было бы
 * малополезно.
 */
export interface LlmCallInfo {
  model: string;
  operation: 'chat' | 'responses';
  outcome: 'success' | 'error';
  durationMs: number;
  /** Эхо `ChatCompletionRequest.requestId`, если был передан — привязывает этот сырой вызов к
   *  конкретному запросу пользователя в логах (см. llm.module.ts), а не только к агрегированной
   *  Prometheus-метрике по модели/операции. */
  requestId?: string;
}

export type LlmCallObserver = (info: LlmCallInfo) => void;

/**
 * Без явного timeout/maxRetries SDK берёт дефолт 10 минут на попытку + 2 ретрая — один
 * зависший/деградировавший вызов OpenAI держит воркер (concurrency=1, см.
 * request-processing.processor.ts) занятым десятками минут, и все остальные запросы в очереди
 * молча стоят за ним без ошибки. 120с с запасом покрывает худший замеренный вызов одного
 * агента (gpt-5.6-terra — до 102с, см. комментарий у DEFAULT_CHAT_MODEL выше) и рвёт зависание
 * достаточно быстро, чтобы пайплайн ушёл в 'failed', а воркер освободился.
 */
const REQUEST_TIMEOUT_MS = 120_000;

export class OpenAiChatClient implements ChatClient {
  private readonly client: OpenAI;

  /**
   * `onCall` — необязательный хук, вызываемый после каждого сырого запроса к OpenAI (успех или
   * ошибка). Сам shared-пакет не знает о Prometheus/prom-client (это зависимость backend, см.
   * llm.module.ts) — поэтому здесь только сырые числа/лейблы через колбэк, а не прямая запись
   * метрики; backend подключает сюда MetricsService.observeLlmCall при создании клиента.
   */
  constructor(
    apiKey: string,
    private readonly onCall?: LlmCallObserver,
  ) {
    this.client = new OpenAI({ apiKey, timeout: REQUEST_TIMEOUT_MS, maxRetries: 1 });
  }

  async complete(request: ChatCompletionRequest): Promise<string> {
    if (request.webSearch) {
      const { content } = await this.completeWithWebSearchMeta(request, request.webSearch);
      return content;
    }

    const model = request.model ?? DEFAULT_CHAT_MODEL;
    const startedAt = Date.now();
    try {
      const response = await this.client.chat.completions.create({
        model,
        ...(request.reasoningEffort ? { reasoning_effort: request.reasoningEffort } : {}),
        messages: [
          { role: 'system', content: request.system },
          { role: 'user', content: request.user },
        ],
      });

      const content = response.choices[0]?.message?.content ?? '';
      if (!content.trim()) {
        // Пустой ответ (контент-фильтр, обрыв, tool-only turn без текста) не должен молча
        // просочиться в пайплайн как "" — вызывающий агент/оркестратор превратит это в 'failed',
        // а не в completed с пустым содержимым.
        throw new Error('OpenAI вернул пустой ответ (chat.completions)');
      }
      this.reportCall(model, 'chat', 'success', startedAt, request.requestId);
      return content;
    } catch (error) {
      this.reportCall(model, 'chat', 'error', startedAt, request.requestId);
      throw error;
    }
  }

  async completeWithWebSearchMeta(
    request: Omit<ChatCompletionRequest, 'webSearch'>,
    webSearch: { allowedDomains: string[] },
  ): Promise<WebSearchResult> {
    const model = request.model ?? DEFAULT_CHAT_MODEL;
    const startedAt = Date.now();
    try {
      const response = await this.client.responses.create({
        model,
        instructions: request.system,
        input: request.user,
        ...(request.previousResponseId ? { previous_response_id: request.previousResponseId } : {}),
        ...(request.reasoningEffort ? { reasoning: { effort: request.reasoningEffort } } : {}),
        tools: [{ type: 'web_search', filters: { allowed_domains: webSearch.allowedDomains } }],
      });

      const content = response.output_text ?? '';
      if (!content.trim()) {
        throw new Error('OpenAI вернул пустой ответ (responses/web_search)');
      }
      this.reportCall(model, 'responses', 'success', startedAt, request.requestId);
      return { content, responseId: response.id, citedUrls: extractCitedUrls(response) };
    } catch (error) {
      this.reportCall(model, 'responses', 'error', startedAt, request.requestId);
      throw error;
    }
  }

  async completeWithWebSearchMetaStream(
    request: Omit<ChatCompletionRequest, 'webSearch'>,
    webSearch: { allowedDomains: string[] },
    onDelta: (delta: string) => void,
  ): Promise<WebSearchResult> {
    const model = request.model ?? DEFAULT_CHAT_MODEL;
    const startedAt = Date.now();
    try {
      const stream = await this.client.responses.create({
        model,
        instructions: request.system,
        input: request.user,
        ...(request.previousResponseId ? { previous_response_id: request.previousResponseId } : {}),
        ...(request.reasoningEffort ? { reasoning: { effort: request.reasoningEffort } } : {}),
        tools: [{ type: 'web_search', filters: { allowed_domains: webSearch.allowedDomains } }],
        stream: true,
      });

      let content = '';
      let responseId = '';
      let citedUrls: string[] = [];
      for await (const event of stream) {
        const typed = event as { type?: string; delta?: unknown; response?: unknown };
        if (typed.type === 'response.output_text.delta' && typeof typed.delta === 'string') {
          content += typed.delta;
          onDelta(typed.delta);
        } else if (typed.type === 'response.completed' && typed.response) {
          const finalResponse = typed.response as {
            id: string;
            output_text?: string;
            output?: unknown;
          };
          responseId = finalResponse.id;
          citedUrls = extractCitedUrls(finalResponse);
          // Фолбэк на случай, если по какой-то причине не пришло ни одной delta (например,
          // модель не написала текстовую часть вообще) — не должны остаться без content там, где
          // он в принципе был в финальном ответе.
          if (!content && finalResponse.output_text) content = finalResponse.output_text;
        }
      }

      if (!content.trim()) {
        throw new Error('OpenAI вернул пустой ответ (responses/web_search, stream)');
      }
      if (!responseId) {
        // Стрим отдал текстовые delta, но событие 'response.completed' так и не пришло (обрыв,
        // либо SDK/сервер не прислали его в этом ответе) — content есть, поэтому выше не упали,
        // но без responseId чейнинг на следующий шаг (previousResponseId, см. orchestrator.ts)
        // молча деградирует: пустая строка отбрасывается условием `... ? {...} : {}` при сборке
        // следующего запроса, и document-агент просто ищет с нуля вместо продолжения диалога —
        // без этого лога это никак не было бы видно (ни ошибки, ни упавшего шага).
        logger.warn(
          { requestId: request.requestId, model },
          'Стрим web_search завершился без response.completed — previousResponseId недоступен для следующего шага',
        );
      }
      this.reportCall(model, 'responses', 'success', startedAt, request.requestId);
      return { content, responseId, citedUrls };
    } catch (error) {
      this.reportCall(model, 'responses', 'error', startedAt, request.requestId);
      throw error;
    }
  }

  private reportCall(
    model: string,
    operation: LlmCallInfo['operation'],
    outcome: LlmCallInfo['outcome'],
    startedAt: number,
    requestId?: string,
  ): void {
    this.onCall?.({
      model,
      operation,
      outcome,
      durationMs: Date.now() - startedAt,
      ...(requestId !== undefined ? { requestId } : {}),
    });
  }
}

function isUrlCitationAnnotation(value: unknown): value is { type: 'url_citation'; url: string } {
  if (typeof value !== 'object' || value === null) return false;
  const record = value as Record<string, unknown>;
  return record.type === 'url_citation' && typeof record.url === 'string';
}

/**
 * OpenAI возвращает реально открытые моделью страницы как `url_citation`-аннотации на
 * message-элементах `response.output` — не задокументированный явно в одном месте формат SDK,
 * поэтому разбираем defensively, а не через строгие типы `openai` SDK, которые точно не
 * гарантируют эту вложенность на все версии ответа. Нужно для чейнинга Агент "answer" → Агент
 * "document" (см. WebSearchResult, completeWithWebSearchMeta) — только ради latency, поэтому
 * пустой результат (SDK поменял форму ответа) не является ошибкой: агент "document" в этом случае
 * просто ищет самостоятельно, как раньше.
 */
function extractCitedUrls(response: { output?: unknown }): string[] {
  const output = Array.isArray(response.output) ? response.output : [];
  const urls = new Set<string>();
  for (const item of output) {
    if (typeof item !== 'object' || item === null) continue;
    const content = (item as Record<string, unknown>).content;
    if (!Array.isArray(content)) continue;
    for (const block of content) {
      if (typeof block !== 'object' || block === null) continue;
      const annotations = (block as Record<string, unknown>).annotations;
      if (!Array.isArray(annotations)) continue;
      for (const annotation of annotations) {
        if (isUrlCitationAnnotation(annotation)) urls.add(annotation.url);
      }
    }
  }
  return [...urls];
}
