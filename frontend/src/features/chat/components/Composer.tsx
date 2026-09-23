import { useRef } from "react";
import type { ChangeEvent, KeyboardEvent } from "react";
import { Mic, Paperclip, Send } from "lucide-react";
import { IconButton } from "@/shared/ui/IconButton";
import { cn } from "@/shared/lib/cn";
import { useAutoResizeTextarea } from "../useAutoResizeTextarea";
import { AttachmentChip } from "./AttachmentChip";
import { CHAT_COLUMN_CLASS } from "../layout";
import type { ChatAttachment } from "../types";
import type { ChatDictionary } from "../locales";

interface ComposerProps {
  t: ChatDictionary;
  draft: string;
  onDraftChange: (text: string) => void;
  attachment: ChatAttachment | null;
  onAttach: (file: File) => void;
  onRemoveAttachment: () => void;
  isRecording: boolean;
  onStartRecording: () => void;
  onStopRecording: () => void;
  onSend: () => void;
  canSend: boolean;
}

/**
 * Всегда виден на экране чата (PLAN.md Stage 1). Позиционирование —
 * контракт из instructions.md: `bottom-[var(--mobile-nav-h)] md:bottom-0`,
 * чтобы не перекрывать нижний таб-бар на мобильном.
 */
export function Composer({
  t,
  draft,
  onDraftChange,
  attachment,
  onAttach,
  onRemoveAttachment,
  isRecording,
  onStartRecording,
  onStopRecording,
  onSend,
  canSend,
}: ComposerProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const textareaRef = useAutoResizeTextarea(draft);

  function handleFileChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (file) onAttach(file);
    event.target.value = "";
  }

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      onSend();
    }
  }

  return (
    <div className="safe-area-bottom fixed inset-x-0 bottom-[var(--mobile-nav-h)] z-composer bg-gradient-to-t from-bg to-transparent px-4 pb-4 pt-8 md:bottom-0">
      <div className={cn(CHAT_COLUMN_CLASS, "flex flex-col gap-2")}>
        {isRecording && (
          <div className="flex h-11 items-center gap-2.5 rounded-xl border border-accent bg-surface px-3.5">
            <span
              aria-hidden="true"
              className="h-2.5 w-2.5 flex-none animate-zan-pulse rounded-full bg-danger"
            />
            <span className="truncate text-caption text-muted">{t.recordingHint}</span>
            <button
              type="button"
              onClick={onStopRecording}
              className="ml-auto h-11 flex-none rounded-md border border-line px-3 text-caption font-semibold text-ink hover:border-accent"
            >
              {t.voiceStopLabel}
            </button>
          </div>
        )}

        {attachment && (
          <AttachmentChip
            name={attachment.name}
            mimeType={attachment.mimeType}
            sizeBytes={attachment.sizeBytes}
            onRemove={onRemoveAttachment}
            removeLabel={t.removeAttachmentLabel}
          />
        )}

        {/* focus-within:border-accent — видимый фокус для textarea (у неё нет своей рамки, focus-visible
            некуда повесить напрямую); тот же паттерн, что Input/Textarea/Select/QuantityStepper используют
            через собственный focus:border-accent (Stage 5 аудит: до этой правки фокус клавиатурой в
            текстовом поле композера ничем не подсвечивался). */}
        <div className="flex items-end gap-1 rounded-2xl border border-line bg-surface p-1.5 shadow-card focus-within:border-accent">
          <IconButton
            variant="ghost"
            aria-label={t.attachLabel}
            onClick={() => fileInputRef.current?.click()}
          >
            <Paperclip size={18} aria-hidden="true" />
          </IconButton>
          <input
            ref={fileInputRef}
            type="file"
            className="hidden"
            onChange={handleFileChange}
          />

          <textarea
            ref={textareaRef}
            rows={1}
            value={draft}
            onChange={(event) => onDraftChange(event.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={t.composerPlaceholder}
            className="max-h-[120px] flex-1 resize-none border-none bg-transparent px-1.5 py-2.5 text-body text-ink outline-none placeholder:text-muted"
          />

          <IconButton
            variant={isRecording ? "accent" : "ghost"}
            aria-label={isRecording ? t.voiceStopLabel : t.voiceStartLabel}
            onClick={isRecording ? onStopRecording : onStartRecording}
          >
            <Mic size={18} aria-hidden="true" />
          </IconButton>

          <IconButton
            variant="accent"
            aria-label={t.sendLabel}
            onClick={onSend}
            disabled={!canSend}
          >
            <Send size={18} aria-hidden="true" />
          </IconButton>
        </div>

        <p className="text-center text-micro font-mono text-muted">
          {t.footerDisclaimer}
        </p>
      </div>
    </div>
  );
}
