import { Skeleton } from "@/shared/ui/Skeleton";

interface TypingIndicatorProps {
  label: string;
}

/** Плейсхолдер ответа, пока идёт мок-задержка (useChatThread — ASSISTANT_REPLY_DELAY_MS). */
export function TypingIndicator({ label }: TypingIndicatorProps) {
  return (
    <div className="max-w-[92%]" aria-live="polite">
      <div className="mb-1.5 font-mono text-micro uppercase text-muted">{label}</div>
      <div className="flex flex-col gap-2 rounded-2xl rounded-tl-sm border border-line bg-surface p-4 shadow-card">
        <Skeleton className="h-3.5 w-2/3" />
        <Skeleton className="h-3.5 w-1/3" />
      </div>
    </div>
  );
}
