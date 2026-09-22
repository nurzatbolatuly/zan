import { QuickTopics } from "./QuickTopics";
import type { Lang } from "@/shared/types/common";
import type { ChatDictionary } from "../locales";

interface ChatEmptyStateProps {
  lang: Lang;
  t: ChatDictionary;
  onSelectTopic: (topicLabel: string, draft: string) => void;
}

/**
 * Герой-экран нового треда (Zan.dc.html:69-74) — не переиспользует shared
 * EmptyState: тот про "нет данных" (пунктирная рамка, компактный блок), это —
 * первый экран продукта (крупный заголовок, без рамки).
 */
export function ChatEmptyState({ lang, t, onSelectTopic }: ChatEmptyStateProps) {
  return (
    <div className="flex max-w-[680px] flex-col gap-8 pb-6 pt-10 sm:pt-14">
      <div>
        <h1 className="mb-2.5 text-display text-ink">{t.emptyTitle}</h1>
        <p className="text-body-lg text-muted">{t.emptySubtitle}</p>
      </div>
      <QuickTopics lang={lang} label={t.quickTopicsLabel} onSelect={onSelectTopic} />
    </div>
  );
}
