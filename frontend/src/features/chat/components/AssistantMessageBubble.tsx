import { ThumbsDown, ThumbsUp } from "lucide-react";
import { cn } from "@/shared/lib/cn";
import { SourcesList } from "./SourcesList";
import type { ChatMessage } from "../types";
import type { MessageFeedbackValue } from "@/shared/types/api";
import type { ChatDictionary } from "../locales";

interface AssistantMessageBubbleProps {
  message: ChatMessage;
  t: ChatDictionary;
  isSourcesOpen: boolean;
  onToggleSources: () => void;
  onVote: (vote: MessageFeedbackValue) => void;
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
  const sourcesPanelId = `chat-sources-${message.id}`;
  const sources = message.sources ?? [];
  const findings = message.findings ?? [];

  return (
    <div className="max-w-[92%]">
      <div className="mb-1.5 flex items-center gap-2 font-mono text-micro uppercase text-muted">
        <span>{t.assistantLabel}</span>
      </div>
      <div className="rounded-2xl rounded-tl-sm border border-line bg-surface p-4 shadow-card">
        {message.text && (
          <p className="mb-2.5 whitespace-pre-wrap text-body text-ink last:mb-0">
            {message.text}
          </p>
        )}

        {findings.length > 0 && (
          <div className="flex flex-col gap-2.5">
            {findings.map((finding) => (
              <p key={finding.title} className="text-body text-ink">
                <strong>{finding.title}.</strong> {finding.body}
              </p>
            ))}
          </div>
        )}

        {sources.length > 0 && (
          <div className="mt-3.5">
            <SourcesList
              sources={sources}
              isOpen={isSourcesOpen}
              onToggle={onToggleSources}
              toggleLabel={t.sourcesToggle}
              panelId={sourcesPanelId}
            />
          </div>
        )}

        <div className="mt-3.5 flex flex-wrap items-center gap-2 border-t border-line pt-3">
          <button
            type="button"
            onClick={() => onVote("like")}
            aria-pressed={message.feedback === "like"}
            className={cn(
              VOTE_BUTTON_BASE,
              message.feedback === "like"
                ? "border-accent bg-accent-soft text-accent"
                : "border-line text-muted hover:border-accent hover:text-accent",
            )}
          >
            <ThumbsUp size={15} aria-hidden="true" />
            {t.helpful}
          </button>
          <button
            type="button"
            onClick={() => onVote("dislike")}
            aria-pressed={message.feedback === "dislike"}
            className={cn(
              VOTE_BUTTON_BASE,
              message.feedback === "dislike"
                ? "border-danger bg-surface-2 text-danger"
                : "border-line text-muted hover:border-accent hover:text-accent",
            )}
          >
            <ThumbsDown size={15} aria-hidden="true" />
            {t.notHelpful}
          </button>
        </div>
      </div>
    </div>
  );
}
