import { useEffect, useRef } from "react";
import { UserMessageBubble } from "./UserMessageBubble";
import { AssistantMessageBubble } from "./AssistantMessageBubble";
import { TypingIndicator } from "./TypingIndicator";
import type { ChatMessage, MessageVote } from "../types";
import type { ChatDictionary } from "../locales";

interface ThreadMessagesProps {
  messages: ChatMessage[];
  isAssistantTyping: boolean;
  expandedSourceMessageIds: ReadonlySet<string>;
  onToggleSources: (messageId: string) => void;
  onVote: (messageId: string, vote: NonNullable<MessageVote>) => void;
  t: ChatDictionary;
}

export function ThreadMessages({
  messages,
  isAssistantTyping,
  expandedSourceMessageIds,
  onToggleSources,
  onVote,
  t,
}: ThreadMessagesProps) {
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // `scrollIntoView({ behavior: "smooth" })` — нативный скролл браузера, глобальный CSS-фолбэк
    // на transition/animation-duration (shared/styles/index.css) его не покрывает — проверяем
    // prefers-reduced-motion явно (PLAN.md Stage 5).
    const prefersReducedMotion = window.matchMedia(
      "(prefers-reduced-motion: reduce)",
    ).matches;
    bottomRef.current?.scrollIntoView({
      behavior: prefersReducedMotion ? "auto" : "smooth",
      block: "end",
    });
  }, [messages.length, isAssistantTyping]);

  return (
    <div className="flex max-w-[760px] flex-col gap-5 pb-6 pt-4">
      {messages.map((message) =>
        message.role === "user" ? (
          <UserMessageBubble key={message.id} message={message} />
        ) : (
          <AssistantMessageBubble
            key={message.id}
            message={message}
            t={t}
            isSourcesOpen={expandedSourceMessageIds.has(message.id)}
            onToggleSources={() => onToggleSources(message.id)}
            onVote={(vote) => onVote(message.id, vote)}
          />
        ),
      )}
      {isAssistantTyping && <TypingIndicator label={t.assistantTypingLabel} />}
      <div ref={bottomRef} />
    </div>
  );
}
