import { ChevronRight } from "lucide-react";
import { cn } from "@/shared/lib/cn";
import type { ChatSource } from "../types";

interface SourcesListProps {
  sources: ChatSource[];
  isOpen: boolean;
  onToggle: () => void;
  toggleLabel: (count: number) => string;
  panelId: string;
}

/** Сворачиваемый блок источников (Zan.dc.html:90-105) — счётчик берётся из данных, не хардкодится. */
export function SourcesList({
  sources,
  isOpen,
  onToggle,
  toggleLabel,
  panelId,
}: SourcesListProps) {
  return (
    <div>
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={isOpen}
        aria-controls={panelId}
        className="flex h-11 w-full items-center gap-2 rounded-md border border-line bg-surface-2 px-3 text-left text-body-sm font-semibold text-ink transition-colors hover:border-accent"
      >
        <ChevronRight
          size={16}
          aria-hidden="true"
          className={cn("transition-transform", isOpen && "rotate-90")}
        />
        <span>{toggleLabel(sources.length)}</span>
      </button>

      {isOpen && (
        <div id={panelId} className="mt-2 flex flex-col gap-2 pl-0.5">
          {sources.map((source) => (
            <div key={source.ref} className="rounded-md border border-line p-3">
              <div className="mb-1 font-mono text-micro text-accent">{source.ref}</div>
              <div className="text-caption text-muted">{source.quote}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
