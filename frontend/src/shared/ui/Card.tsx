import type { HTMLAttributes } from "react";
import { cn } from "@/shared/lib/cn";

type CardTone = "default" | "spotlight";

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  /** "spotlight" — анимированный градиент-микс (`.tone-spotlight`,
   * FRONT_DESIGN_SYSTEM.md §1), для одной "цепляющей взгляд" карточки на
   * экране (например, "свой набор" среди тарифов), не для массового использования. */
  tone?: CardTone;
}

// Цвет рамки — часть TONES, а не отдельный базовый класс: два класса на одно
// CSS-свойство (`border-line` + `border-accent`) резолвились бы порядком в
// сгенерированном CSS, а не порядком в строке — см. предупреждение в cn.ts.
// `tone-spotlight` — кастомный CSS-класс (shared/styles/index.css), не
// Tailwind-утилита: анимированный градиент не выразить одной bg-* утилитой.
const TONES: Record<CardTone, string> = {
  default: "border-line bg-surface",
  spotlight: "border-accent tone-spotlight",
};

export function Card({ className, tone = "default", ...props }: CardProps) {
  return (
    <div
      className={cn("rounded-xl border p-4 shadow-card", TONES[tone], className)}
      {...props}
    />
  );
}
