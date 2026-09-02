import { Redis } from 'ioredis';
import OpenAI from 'openai';
import type { SupabaseClient } from '@supabase/supabase-js';
import { createLogger, type Logger } from '@zan/shared';
import { parseRedisConnection } from './common/redis-connection.js';

/**
 * Пользователь жаловался, что при недоступном сервисе (например, неверный OPENAI_API_KEY)
 * непонятно, "завис ли агент или ключ неверный" — запрос молча висит в pending/processing, а
 * причина всплывает только когда кто-то реально отправит вопрос и дождётся ошибки. Эта проверка
 * запускается один раз при старте backend (см. main.ts), до того как он начнёт принимать
 * трафик, и явно фейлит запуск, если Supabase/Redis/OpenAI недоступны — вместо тихого "зависания"
 * первого реального запроса пользователя.
 */
const CHECK_TIMEOUT_MS = 5000;

export interface StartupCheck {
  service: string;
  run: () => Promise<void>;
}

export interface StartupCheckResult {
  service: string;
  ok: boolean;
  error?: string;
}

async function withTimeout<T>(promise: Promise<T>, ms: number, label: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout>;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => reject(new Error(`${label}: превышен тайм-аут ${ms}мс`)), ms);
  });
  try {
    return await Promise.race([promise, timeout]);
  } finally {
    clearTimeout(timer!);
  }
}

/** Ядро проверки — чистая функция без реального I/O внутри, поэтому тестируется напрямую
 *  фейковыми `run()` (в т.ч. "зависающими"), без мока ioredis/openai/supabase. */
export async function evaluateStartupChecks(checks: StartupCheck[]): Promise<StartupCheckResult[]> {
  return Promise.all(
    checks.map(async ({ service, run }) => {
      try {
        await withTimeout(run(), CHECK_TIMEOUT_MS, service);
        return { service, ok: true };
      } catch (error) {
        return {
          service,
          ok: false,
          error: error instanceof Error ? error.message : String(error),
        };
      }
    }),
  );
}

export function formatStartupFailure(results: StartupCheckResult[]): string | null {
  const failed = results.filter((r) => !r.ok);
  if (failed.length === 0) return null;
  return `Недоступны необходимые сервисы при запуске: ${failed
    .map((f) => `${f.service} (${f.error})`)
    .join('; ')}`;
}

async function checkSupabase(supabase: SupabaseClient): Promise<void> {
  const { error } = await supabase.from('requests').select('id').limit(1);
  if (error) throw new Error(error.message);
}

async function checkRedis(redisUrl: string): Promise<void> {
  const redis = new Redis({
    ...parseRedisConnection(redisUrl),
    lazyConnect: true,
    maxRetriesPerRequest: 1,
    // Разовая диагностическая проверка — не нужен бесконечный автопереподключение ioredis по
    // умолчанию, оно бы просто "проглотило" сбой вместо быстрого reject.
    retryStrategy: () => null,
  });
  try {
    await redis.connect();
    await redis.ping();
  } finally {
    redis.disconnect();
  }
}

async function checkOpenAi(apiKey: string): Promise<void> {
  const client = new OpenAI({ apiKey });
  await client.models.list();
}

export function buildStartupChecks(deps: {
  supabase: SupabaseClient;
  redisUrl: string;
  openAiApiKey: string;
}): StartupCheck[] {
  return [
    { service: 'Supabase', run: () => checkSupabase(deps.supabase) },
    { service: 'Redis', run: () => checkRedis(deps.redisUrl) },
    { service: 'OpenAI', run: () => checkOpenAi(deps.openAiApiKey) },
  ];
}

/**
 * Точка входа, вызывается из main.ts до app.listen(). Бросает исключение, если хоть один
 * сервис недоступен, — main.ts на это явно останавливает запуск (см. комментарий там про
 * app.close()/process.exit).
 */
export async function runStartupChecks(deps: {
  supabase: SupabaseClient;
  redisUrl: string;
  openAiApiKey: string;
}): Promise<void> {
  const logger: Logger = createLogger('startup-checks');
  const results = await evaluateStartupChecks(buildStartupChecks(deps));

  for (const result of results) {
    if (result.ok) {
      logger.info({ service: result.service }, 'Сервис доступен');
    } else {
      logger.error(
        { service: result.service, error: result.error },
        'Сервис недоступен при запуске backend',
      );
    }
  }

  const failureMessage = formatStartupFailure(results);
  if (failureMessage) {
    throw new Error(failureMessage);
  }
}
