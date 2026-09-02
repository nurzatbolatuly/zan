import OpenAI from 'openai';

/**
 * Общий клиент для текстовых LLM-вызовов OpenAI — понадобится всем 4 агентам
 * (Агент 1 — с Этапа 3, Агенты 2–4 — на последующих этапах), поэтому вынесен сюда
 * сразу, а не скопирован по одному разу на агента.
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
   * 1, 2 и 4 (см. law-search.agent.ts, law-verification.agent.ts, law-document.agent.ts) —
   * каждый живьём читает adilet.zan.kz/zan.gov.kz на каждый свой вызов, локального индекса нет
   * (см. INSTRUCTIONS.md §9 — решение вернуться к live-поиску после отказа от retrieval-корпуса).
   * Присутствие этого поля переключает вызов на Responses API вместо Chat Completions — у
   * обычных вызовов без него поведение не меняется.
   */
  webSearch?: { allowedDomains: string[] };
}

export interface ChatClient {
  complete(request: ChatCompletionRequest): Promise<string>;
}

/**
 * Один сырой вызов OpenAI — снаружи ChatClient (агенты, оркестратор) виден только итоговый
 * текст ответа, поэтому именно здесь, у самой границы с сетевым вызовом, единственное место,
 * где можно замерить его реальную задержку. `operation` различает Chat Completions
 * ('chat') и Responses API с веб-поиском ('responses', см. completeWithWebSearch) — это разные
 * эндпоинты OpenAI с разным профилем задержки, схлопывать их в одну метрику было бы
 * малополезно.
 */
export interface LlmCallInfo {
  model: string;
  operation: 'chat' | 'responses';
  outcome: 'success' | 'error';
  durationMs: number;
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
      return this.completeWithWebSearch(request, request.webSearch);
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
      this.reportCall(model, 'chat', 'success', startedAt);
      return content;
    } catch (error) {
      this.reportCall(model, 'chat', 'error', startedAt);
      throw error;
    }
  }

  private async completeWithWebSearch(
    request: ChatCompletionRequest,
    webSearch: { allowedDomains: string[] },
  ): Promise<string> {
    const model = request.model ?? DEFAULT_CHAT_MODEL;
    const startedAt = Date.now();
    try {
      const response = await this.client.responses.create({
        model,
        instructions: request.system,
        input: request.user,
        ...(request.reasoningEffort ? { reasoning: { effort: request.reasoningEffort } } : {}),
        tools: [{ type: 'web_search', filters: { allowed_domains: webSearch.allowedDomains } }],
      });

      const content = response.output_text ?? '';
      if (!content.trim()) {
        throw new Error('OpenAI вернул пустой ответ (responses/web_search)');
      }
      this.reportCall(model, 'responses', 'success', startedAt);
      return content;
    } catch (error) {
      this.reportCall(model, 'responses', 'error', startedAt);
      throw error;
    }
  }

  private reportCall(
    model: string,
    operation: LlmCallInfo['operation'],
    outcome: LlmCallInfo['outcome'],
    startedAt: number,
  ): void {
    this.onCall?.({ model, operation, outcome, durationMs: Date.now() - startedAt });
  }
}
