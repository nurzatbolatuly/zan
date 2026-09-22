import { forwardRef } from "react";
import type { ButtonHTMLAttributes } from "react";
import { cn } from "@/shared/lib/cn";

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** Обязателен — иконка без подписи недоступна для скринридера (FRONT_DESIGN_SYSTEM.md §8). */
  "aria-label": string;
  variant?: "default" | "danger-hover" | "accent" | "ghost" | "ghost-danger";
}

/**
 * Единственный размер — 44×44px (FRONT_DESIGN_SYSTEM.md §9, минимальный touch-таргет).
 * Намеренно без пропа size: прототип использовал иконки-кнопки по 28-30px
 * (Zan.dc.html:327-335) — этого не повторяем, а не просто "не рекомендуем".
 */
export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(
  ({ className, variant = "default", type = "button", ...props }, ref) => (
    <button
      ref={ref}
      type={type}
      className={cn(
        "grid h-11 w-11 place-items-center rounded-md border transition-colors",
        "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
        "disabled:cursor-not-allowed disabled:opacity-50",
        variant === "default" &&
          "border-line bg-surface text-muted hover:border-accent hover:text-accent",
        variant === "danger-hover" &&
          "border-line bg-surface text-muted hover:border-danger hover:text-danger",
        // "accent" и "ghost" — обе без видимой рамки: обе всегда используются
        // внутри уже обрамлённого контейнера (композер чата — вложение/голос/
        // отправка сидят внутри общей рамки инпута), вторая рамка поверх контейнера
        // лишняя (на активной записи голоса выглядела как двойной контур).
        variant === "accent" &&
          "border-transparent bg-accent text-accent-ink hover:opacity-90",
        variant === "ghost" &&
          "border-transparent bg-transparent text-muted hover:bg-surface-2 hover:text-ink",
        // Как "ghost", но hover красный — для удаления внутри уже обрамлённого блока
        // (карточка треда History и т.п.), где своя рамка на кнопке не нужна, а сигнал
        // "это разрушительное действие" — нужен. bg-danger/10 не работает (см. Badge.tsx —
        // Tailwind не режет alpha у цвета из CSS var), поэтому фон при hover — surface-2.
        variant === "ghost-danger" &&
          "border-transparent bg-transparent text-muted hover:bg-surface-2 hover:text-danger",
        className,
      )}
      {...props}
    />
  ),
);
IconButton.displayName = "IconButton";
