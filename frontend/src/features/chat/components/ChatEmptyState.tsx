import { cn } from "@/shared/lib/cn";
import { CHAT_COLUMN_CLASS } from "../layout";
import { QuickTopics } from "./QuickTopics";
import type { ChatDictionary } from "../locales";

interface ChatEmptyStateProps {
  t: ChatDictionary;
  onSelectTopic: (topicLabel: string, draft: string) => void;
}

/**
 * Герой-экран нового треда (Zan.dc.html:69-74) — не переиспользует shared
 * EmptyState: тот про "нет данных" (пунктирная рамка, компактный блок), это —
 * первый экран продукта (крупный заголовок, без рамки). Стоит в общей
 * колонке чата (layout.ts) — ровно над полем ввода; 680px — только ширина
 * строки заголовка/подзаголовка, чтобы крупный текст не растягивался.
 */
export function ChatEmptyState({ t, onSelectTopic }: ChatEmptyStateProps) {
  return (
    <div className={cn(CHAT_COLUMN_CLASS, "flex flex-col gap-8 pb-6 pt-10 sm:pt-14")}>
      <div className="max-w-[680px]">
        <h1 className="mb-2.5 text-display text-ink">{t.emptyTitle}</h1>
        <p className="text-body-lg text-muted">{t.emptySubtitle}</p>
      </div>
      <QuickTopics
        label={t.quickTopicsLabel}
        topics={t.quickTopics}
        onSelect={onSelectTopic}
      />
    </div>
  );
}
