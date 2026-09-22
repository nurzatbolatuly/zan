import type { ReactNode } from "react";
import { cn } from "@/shared/lib/cn";

interface EmptyStateProps {
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}

export function EmptyState({ title, description, action, className }: EmptyStateProps) {
  return (
    <div
      className={cn(
        "rounded-xl border border-dashed border-line px-6 py-12 text-center",
        className,
      )}
    >
      <div className="mb-2 text-h3 text-ink">{title}</div>
      {description && (
        <p className="mx-auto mb-4 max-w-sm text-body-sm text-muted">{description}</p>
      )}
      {action}
    </div>
  );
}
