import { forwardRef } from "react";
import type { InputHTMLAttributes } from "react";
import { cn } from "@/shared/lib/cn";

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  hint?: string;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, label, hint, ...props }, ref) => (
    <label className="block">
      {label && (
        <span className="mb-1 block text-body-sm font-semibold text-ink">{label}</span>
      )}
      <input
        ref={ref}
        className={cn(
          "h-11 w-full rounded-md border border-line bg-surface px-3 text-body text-ink outline-none",
          "placeholder:text-muted focus:border-accent",
          className,
        )}
        {...props}
      />
      {hint && <span className="mt-1 block text-caption text-muted">{hint}</span>}
    </label>
  ),
);
Input.displayName = "Input";
