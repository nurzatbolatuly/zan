import { useEffect, useRef } from "react";
import { UserMessageBubble } from "./UserMessageBubble";
import { AssistantMessageBubble } from "./AssistantMessageBubble";
import { StreamingAssistantBubble } from "./StreamingAssistantBubble";
import { ReplyProgressIndicator } from "./ReplyProgressIndicator";
import { cn } from "@/shared/lib/cn";
import { CHAT_COLUMN_CLASS } from "../layout";
import type { ChatMessage } from "../types";
import type { ReplyProgressStep } from "../replyProgress";
import type { MessageFeedbackValue } from "@/shared/types/api";
import type { ChatDictionary } from "../locales";

interface ThreadMessagesProps {
  messages: ChatMessage[];
  /** Этап ожидания ответа до первого токена стрима; null — ответ не ожидается. */
  replyProgress: ReplyProgressStep | null;
  /** Накопленный текст текущего стрима (WS answer_delta); null — ничего не льётся. */
  streamingText: string | null;
  streamingMessageId: string | null;
  expandedSourceMessageIds: ReadonlySet<string>;
  onToggleSources: (messageId: string) => void;
  onVote: (messageId: string, vote: MessageFeedbackValue) => void;
  t: ChatDictionary;
}

export function ThreadMessages({
  messages,
  replyProgress,
  streamingText,
  streamingMessageId,
  expandedSourceMessageIds,
  onToggleSources,
  onVote,
  t,
}: ThreadMessagesProps) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const isAwaitingReply = replyProgress !== null;

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
    // streamingText?.length — держит автоскролл внизу, пока текст растёт токен за токеном, не только на новое сообщение.
  }, [messages.length, isAwaitingReply, streamingText?.length]);

  return (
    <div className={cn(CHAT_COLUMN_CLASS, "flex flex-col gap-5 pb-6 pt-4")}>
      {messages.map((message) =>
        message.sender === "user" ? (
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
      {streamingText !== null ? (
        <StreamingAssistantBubble
          key={streamingMessageId ?? "streaming"}
          text={streamingText}
          t={t}
        />
      ) : (
        isAwaitingReply && <ReplyProgressIndicator step={replyProgress} t={t} />
      )}
      <div ref={bottomRef} />
    </div>
  );
}
