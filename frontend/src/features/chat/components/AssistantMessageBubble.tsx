import { ThumbsDown, ThumbsUp } from "lucide-react";
import { cn } from "@/shared/lib/cn";
import { useLangStore } from "@/shared/stores/useLangStore";
import { getAssistantReplyContent } from "../mocks";
import { SourcesList } from "./SourcesList";
import { DocumentCard } from "./DocumentCard";
import type { AssistantChatMessage } from "../types";
import type { ChatDictionary } from "../locales";

interface AssistantMessageBubbleProps {
  message: AssistantChatMessage;
  t: ChatDictionary;
  isSourcesOpen: boolean;
  onToggleSources: () => void;
  onVote: (vote: "up" | "down") => void;
}

const VOTE_BUTTON_BASE =
  "flex h-11 items-center gap-1.5 rounded-md border px-3 text-body-sm font-semibold transition-colors";

export function AssistantMessageBubble({
  message,
  t,
  isSourcesOpen,
  onToggleSources,
  onVote,
}: AssistantMessageBubbleProps) {
  const lang = useLangStore((state) => state.lang);
  const content = getAssistantReplyContent(message.replyKind, lang);
  const sourcesPanelId = `chat-sources-${message.id}`;

  return (
    <div className="max-w-[92%]">
      <div className="mb-1.5 font-mono text-micro uppercase text-muted">
        {content.metaLabel}
      </div>
      <div className="rounded-2xl rounded-tl-sm border border-line bg-surface p-4 shadow-card">
        {content.paragraphs.map((paragraph) => (
          <p key={paragraph} className="mb-2.5 text-body text-ink last:mb-0">
            {paragraph}
          </p>
        ))}

        {content.findings.length > 0 && (
          <div className="flex flex-col gap-2.5">
            {content.findings.map((finding) => (
              <p key={finding.title} className="text-body text-ink">
                <strong>{finding.title}.</strong> {finding.body}
              </p>
            ))}
          </div>
        )}

        {content.sources.length > 0 && (
          <div className="mt-3.5">
            <SourcesList
              sources={content.sources}
              isOpen={isSourcesOpen}
              onToggle={onToggleSources}
              toggleLabel={t.sourcesToggle}
              panelId={sourcesPanelId}
            />
          </div>
        )}

        {content.document && (
          <div className="mt-3.5">
            <DocumentCard document={content.document} t={t} />
          </div>
        )}

        {content.document === null && (
          <div className="mt-3.5 flex flex-wrap items-center gap-2 border-t border-line pt-3">
            <button
              type="button"
              onClick={() => onVote("up")}
              aria-pressed={message.vote === "up"}
              className={cn(
                VOTE_BUTTON_BASE,
                message.vote === "up"
                  ? "border-accent bg-accent-soft text-accent"
                  : "border-line text-muted hover:border-accent hover:text-accent",
              )}
            >
              <ThumbsUp size={15} aria-hidden="true" />
              {t.helpful}
            </button>
            <button
              type="button"
              onClick={() => onVote("down")}
              aria-pressed={message.vote === "down"}
              className={cn(
                VOTE_BUTTON_BASE,
                message.vote === "down"
                  ? "border-danger bg-surface-2 text-danger"
                  : "border-line text-muted hover:border-accent hover:text-accent",
              )}
            >
              <ThumbsDown size={15} aria-hidden="true" />
              {t.notHelpful}
            </button>
            <div className="ml-auto font-mono text-micro uppercase text-muted">
              {t.freeFollowup}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
