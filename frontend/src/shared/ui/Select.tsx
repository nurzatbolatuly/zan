import { forwardRef } from "react";
import type { SelectHTMLAttributes } from "react";
import { cn } from "@/shared/lib/cn";

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({ className, label, children, ...props }, ref) => (
    <label className="block">
      {label && (
        <span className="mb-1 block text-body-sm font-semibold text-ink">{label}</span>
      )}
      <select
        ref={ref}
        className={cn(
          "h-11 w-full cursor-pointer rounded-md border border-line bg-surface px-3 text-body text-ink outline-none",
          "focus:border-accent",
          className,
        )}
        {...props}
      >
        {children}
      </select>
    </label>
  ),
);
Select.displayName = "Select";
