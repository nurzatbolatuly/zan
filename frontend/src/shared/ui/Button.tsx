import { forwardRef } from "react";
import type { ButtonHTMLAttributes } from "react";
import { cn } from "@/shared/lib/cn";

type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
type ButtonSize = "md" | "sm";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
}

const BASE =
  "inline-flex items-center justify-center gap-2 rounded-md font-semibold transition-colors " +
  "disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline focus-visible:outline-2 " +
  "focus-visible:outline-offset-2 focus-visible:outline-accent";

const VARIANTS: Record<ButtonVariant, string> = {
  primary: "bg-accent text-accent-ink hover:opacity-90",
  secondary: "border border-line bg-transparent text-ink hover:border-accent",
  ghost: "bg-transparent text-muted hover:text-ink",
  danger: "bg-danger text-white hover:opacity-90",
};

// FRONT_DESIGN_SYSTEM.md §9 — минимум 44px для ЛЮБОГО интерактивного элемента,
// без исключения для "компактных" кнопок. `sm` отличается от `md` только
// паддингом/размером текста, высота у обоих одна — h-11 (Stage 5 аудит: до
// этой правки `sm` был h-9/36px, ниже минимума — see BundlesSection).
const SIZES: Record<ButtonSize, string> = {
  md: "h-11 px-4 text-body-sm",
  sm: "h-11 px-3 text-caption",
};

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = "primary", size = "md", type = "button", ...props }, ref) => (
    <button
      ref={ref}
      type={type}
      className={cn(BASE, VARIANTS[variant], SIZES[size], className)}
      {...props}
    />
  ),
);
Button.displayName = "Button";
