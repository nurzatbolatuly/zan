import type { ChatDictionary } from "../locales";

interface StreamingAssistantBubbleProps {
  text: string;
  t: ChatDictionary;
}

/**
 * Та же визуальная оболочка, что AssistantMessageBubble, но для ещё не
 * завершённого сообщения (streaming-вызов LLM — токены летят по WS в
 * реальном времени, `answer_delta`). Без sources/findings/feedback —
 * их не существует до `answer_done`. AssistantMessageBubble
 * сам не меняется — рендерит только готовые MessageDto, партиал-режим в
 * него не протекает (чище граница, чем один компонент на оба случая).
 */
export function StreamingAssistantBubble({ text, t }: StreamingAssistantBubbleProps) {
  return (
    <div className="max-w-[92%]">
      <div className="mb-1.5 flex items-center gap-2 font-mono text-micro uppercase text-muted">
        <span>{t.assistantLabel}</span>
      </div>
      <div className="rounded-2xl rounded-tl-sm border border-line bg-surface p-4 shadow-card">
        <p className="whitespace-pre-wrap text-body text-ink">
          {text}
          <span
            className="ml-0.5 inline-block h-4 w-[2px] translate-y-0.5 animate-pulse bg-ink"
            aria-hidden="true"
          />
        </p>
      </div>
    </div>
  );
}
