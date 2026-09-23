/**
 * Этап ожидания ответа ассистента — что показать пользователю между
 * отправкой и первым токеном стрима (`answer_delta`).
 *
 * Бэк не шлёт промежуточных этапов обработки: по WS приходит только
 * `thread_status(processing)`, а reasoning-модель может молчать до первого
 * токена десятки секунд (instructions.md §8 «Reasoning-модели»). Поэтому
 * этап — реальный там, где фронт его знает (`sending` — POST в полёте,
 * `readingFile` — к вопросу приложен файл, бэк его извлекает), дальше —
 * по прошедшему времени. Последний этап открытый: честно говорит, что
 * ответ ещё готовится, вместо бесконечного «печатает» без текста.
 */
export type ReplyProgressStep =
  "sending" | "readingFile" | "analyzing" | "checkingLaw" | "composing" | "takingLonger";

interface TimedStep {
  step: ReplyProgressStep;
  durationMs: number;
}

const FILE_STEP: TimedStep = { step: "readingFile", durationMs: 6_000 };

const PROCESSING_STEPS: readonly TimedStep[] = [
  { step: "analyzing", durationMs: 5_000 },
  { step: "checkingLaw", durationMs: 9_000 },
  { step: "composing", durationMs: 16_000 },
];

const FINAL_STEP: ReplyProgressStep = "takingLonger";

export interface ReplyProgressInput {
  isSending: boolean;
  hasFile: boolean;
  /** Сколько прошло с начала ожидания ответа. */
  elapsedMs: number;
}

export function resolveReplyProgressStep({
  isSending,
  hasFile,
  elapsedMs,
}: ReplyProgressInput): ReplyProgressStep {
  if (isSending) return "sending";

  const steps = hasFile ? [FILE_STEP, ...PROCESSING_STEPS] : PROCESSING_STEPS;
  let stepEndsAtMs = 0;
  for (const { step, durationMs } of steps) {
    stepEndsAtMs += durationMs;
    if (elapsedMs < stepEndsAtMs) return step;
  }
  return FINAL_STEP;
}
