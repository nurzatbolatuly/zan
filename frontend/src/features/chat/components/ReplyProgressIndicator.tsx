import { Skeleton } from "@/shared/ui/Skeleton";
import type { ReplyProgressStep } from "../replyProgress";
import type { ChatDictionary } from "../locales";

interface ReplyProgressIndicatorProps {
  step: ReplyProgressStep;
  t: ChatDictionary;
}

/**
 * Плейсхолдер ответа ассистента до первого токена стрима: та же оболочка,
 * что StreamingAssistantBubble (пузырь не «прыгает», когда пойдёт текст),
 * внутри — текущий этап обработки (replyProgress.ts), а не безликое
 * «печатает», за которым долго ничего не появляется.
 */
export function ReplyProgressIndicator({ step, t }: ReplyProgressIndicatorProps) {
  return (
    <div className="max-w-[92%]">
      <div className="mb-1.5 font-mono text-micro uppercase text-muted">
        {t.assistantLabel}
      </div>
      <div className="flex flex-col gap-3 rounded-2xl rounded-tl-sm border border-line bg-surface p-4 shadow-card">
        <p role="status" className="flex items-center gap-2 text-body-sm text-muted">
          <span
            className="inline-block h-2 w-2 shrink-0 animate-pulse rounded-pill bg-accent"
            aria-hidden="true"
          />
          {t.replyProgress[step]}
        </p>
        <Skeleton className="h-3.5 w-2/3" />
        <Skeleton className="h-3.5 w-1/3" />
      </div>
    </div>
  );
}
