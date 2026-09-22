import { forwardRef } from "react";
import type { TextareaHTMLAttributes } from "react";
import { cn } from "@/shared/lib/cn";

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string;
  hint?: string;
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, label, hint, ...props }, ref) => (
    <label className="block">
      {label && (
        <span className="mb-1 block text-body-sm font-semibold text-ink">{label}</span>
      )}
      {hint && <span className="mb-2 block text-caption text-muted">{hint}</span>}
      <textarea
        ref={ref}
        className={cn(
          "w-full resize-y rounded-md border border-line bg-surface px-3 py-2 text-body text-ink outline-none",
          "placeholder:text-muted focus:border-accent",
          className,
        )}
        {...props}
      />
    </label>
  ),
);
Textarea.displayName = "Textarea";
