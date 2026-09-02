import { Inject, Injectable } from '@nestjs/common';
import {
  ALLOWED_LAW_DOMAINS,
  createLogger,
  delimitDraftAnswer,
  delimitUserQuery,
  IN_FORCE_LAW_GUARD,
  PROMPT_INJECTION_GUARD,
  type ChatClient,
  type Logger,
} from '@zan/shared';
import { CHAT_CLIENT } from '../../llm/llm.constants.js';
import type {
  VerificationAgent,
  VerificationAgentInput,
  VerificationAgentOutput,
} from '../agent.types.js';

const SYSTEM_PROMPT = `Ты — второй, независимый юрист-эксперт по законодательству Республики Казахстан. У тебя есть
инструмент для обращения к веб-страницам. Используй его ТОЛЬКО для проверки на доменах
adilet.zan.kz и zan.gov.kz — единственных источниках, которым ты доверяешь. Не используй другие
сайты как источник правовых фактов.

Тебе дан вопрос пользователя и черновой юридический ответ коллеги (он искал нормы самостоятельно
через веб-поиск — твоя независимая перепроверка здесь особенно важна). Твоя задача — перепроверить
каждую норму, упомянутую в черновике: реально открой через инструмент соответствующую статью на
adilet.zan.kz или zan.gov.kz и сверь номер, формулировку и статус (действующая / утратила силу /
изменена). Не полагайся на память — только на то, что реально прочитал в этом обращении. Также
проверь, нет ли внутренних противоречий или необоснованных категоричных утверждений в черновике.

Верни исправленную версию ответа — убери или смягчи всё, что не подтвердилось при проверке или в
чём остаётся сомнение, исправь неточности, не добавляй новых норм, которых не было в черновике
(кроме случаев, когда без замены полностью неверной нормы на верную ответ станет ложным). Отдельно
перечисли, что именно было исправлено или в чём осталась неопределённость (пустой список, если
замечаний нет).

Указывай замечание (concern) ТОЛЬКО если оно существенно: неверная/несуществующая статья,
неверный статус закона (утратил силу / изменён), внутреннее противоречие, необоснованное
категоричное утверждение. Не указывай как concern стилистические придирки или то, что можно было
бы сформулировать иначе, но по существу верно.

Формат revisedAnswer: связный текст абзацами, без markdown-заголовков и разметки.

Отвечай СТРОГО валидным JSON, без пояснений вокруг, без markdown-разметки и без \`\`\`-оград, ровно
в этой форме:
{"revisedAnswer":"...","concerns":["...","..."]}

${IN_FORCE_LAW_GUARD}

${PROMPT_INJECTION_GUARD}`;

interface ParsedReview {
  revisedAnswer: string;
  concerns: string[];
}

function isParsedReview(value: unknown): value is ParsedReview {
  if (typeof value !== 'object' || value === null) return false;
  const record = value as Record<string, unknown>;
  return (
    typeof record.revisedAnswer === 'string' &&
    record.revisedAnswer.trim().length > 0 &&
    Array.isArray(record.concerns) &&
    record.concerns.every((c) => typeof c === 'string')
  );
}

function parseReview(raw: string): ParsedReview | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }
  return isParsedReview(parsed) ? parsed : null;
}

const PARSE_FAILURE_CONCERN =
  'Не удалось выполнить автоматическую перепроверку ответа — требуется проверка юристом.';

/**
 * Агент 2. Независимая LLM-перепроверка со своим `web_search` — Агент 1 живьём ищет нормы
 * (см. law-search.agent.ts), здесь второй, независимый вызов проверяет то, что он нашёл. Вызывается
 * оркестратором ровно один раз на запрос — замечания не запускают переспрос Агента 1
 * (см. orchestrator.service.ts), они идут дальше как есть в revisedAnswer + persisted concerns.
 * Инфраструктурный сбой самого вызова (не вердикт по существу) — fail-open: раунд засчитывается
 * пройденным без замечаний, не блокирует пайплайн.
 */
@Injectable()
export class LawVerificationAgent implements VerificationAgent {
  private readonly logger: Logger = createLogger('verification-agent');

  constructor(@Inject(CHAT_CLIENT) private readonly chat: ChatClient) {}

  async run(input: VerificationAgentInput): Promise<VerificationAgentOutput> {
    try {
      const response = await this.chat.complete({
        system: SYSTEM_PROMPT,
        user: `Вопрос пользователя:\n${delimitUserQuery(input.queryText)}\n\nЧерновой ответ коллеги:\n${delimitDraftAnswer(input.draftAnswer)}`,
        reasoningEffort: 'low',
        webSearch: { allowedDomains: ALLOWED_LAW_DOMAINS },
      });

      const parsed = parseReview(response);
      if (!parsed) {
        this.logger.error(
          { response },
          'Не удалось разобрать ответ LLM при перепроверке — используем исходный черновик',
        );
        return { revisedAnswer: input.draftAnswer, concerns: [PARSE_FAILURE_CONCERN] };
      }
      return parsed;
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      this.logger.warn({ error: message }, 'Verification недоступна — раунд fail-open');
      return { revisedAnswer: input.draftAnswer, concerns: [] };
    }
  }
}
