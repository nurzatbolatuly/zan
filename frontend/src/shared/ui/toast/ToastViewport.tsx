import { useToastStore } from "./useToastStore";
import type { ToastTone } from "./useToastStore";
import { cn } from "@/shared/lib/cn";

const TONE_CLASS: Record<ToastTone, string> = {
  info: "border-line bg-surface text-ink",
  success: "border-accent bg-accent-soft text-accent",
  error: "border-danger bg-surface text-danger",
};

/**
 * Смонтирован один раз в AppShell — страницы просто зовут useToast().
 * Без кнопки закрытия — тост короткий (2с, см. AUTO_DISMISS_MS в useToastStore.ts)
 * и всегда уходит сам, отдельный контрол для этого не нужен.
 */
export function ToastViewport() {
  const toasts = useToastStore((state) => state.toasts);

  if (toasts.length === 0) return null;

  return (
    <div className="fixed inset-x-0 top-4 z-toast flex flex-col items-center gap-2 px-4">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          role="status"
          className={cn(
            "w-full max-w-sm rounded-lg border px-4 py-3 text-center text-body-sm shadow-card",
            TONE_CLASS[toast.tone],
          )}
        >
          {toast.message}
        </div>
      ))}
    </div>
  );
}
