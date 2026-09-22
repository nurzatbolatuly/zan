import type { HTMLAttributes } from "react";
import { cn } from "@/shared/lib/cn";

type BadgeTone = "accent" | "warn" | "danger" | "muted";

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: BadgeTone;
}

// Паттерн статус-бейджа из прототипа (Zan.dc.html:898): фон всегда --surface-2,
// цвет текста меняется по тону. Для danger/muted нет отдельного "мягкого" фона —
// не придумываем его через opacity-модификатор поверх CSS-переменной (Tailwind
// не умеет резать alpha у произвольных var()-цветов), а используем то, что
// уже есть в токенах (FRONT_DESIGN_SYSTEM.md §1).
const TONES: Record<BadgeTone, string> = {
  accent: "bg-accent-soft text-accent",
  warn: "bg-warn-soft text-warn",
  danger: "bg-surface-2 text-danger",
  muted: "bg-surface-2 text-muted",
};

export function Badge({ className, tone = "muted", ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-pill px-2.5 py-1 font-mono text-micro uppercase",
        TONES[tone],
        className,
      )}
      {...props}
    />
  );
}
