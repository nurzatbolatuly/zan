import type { QuickTopic } from "../locales";

interface QuickTopicsProps {
  label: string;
  topics: QuickTopic[];
  onSelect: (topicLabel: string, draft: string) => void;
}

/** Чипы-подсказки на пустом экране (gap PLAN.md §2, brief 3.1) — клик подставляет черновик в композер. */
export function QuickTopics({ label, topics, onSelect }: QuickTopicsProps) {
  return (
    <div>
      <div className="mb-2 font-mono text-micro uppercase text-muted">{label}</div>
      <div className="flex flex-wrap gap-2">
        {topics.map((topic) => (
          <button
            key={topic.label}
            type="button"
            onClick={() => onSelect(topic.label, topic.draft)}
            className="h-11 rounded-pill border border-line bg-surface px-4 text-body-sm font-semibold text-ink transition-colors hover:border-accent hover:text-accent"
          >
            {topic.label}
          </button>
        ))}
      </div>
    </div>
  );
}
