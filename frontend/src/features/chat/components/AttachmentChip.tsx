import { File, FileText, FileType2, Image, X } from "lucide-react";
import { IconButton } from "@/shared/ui/IconButton";
import { formatFileSize } from "@/shared/lib/format";
import { detectAttachmentKind } from "../attachments";
import type { AttachmentKind } from "../types";

const KIND_ICON: Record<AttachmentKind, typeof FileText> = {
  pdf: FileText,
  docx: FileType2,
  image: Image,
  other: File,
};

/** Отображаемые поля файла — общие для вложения в composer и в отправленном сообщении. */
interface AttachmentChipFile {
  name: string;
  mimeType: string;
  sizeBytes: number;
}

type AttachmentChipProps =
  | (AttachmentChipFile & {
      /** Без onRemove — просто карточка в отправленном сообщении (не съёмная). */
      onRemove?: undefined;
      removeLabel?: undefined;
    })
  | (AttachmentChipFile & {
      onRemove: () => void;
      /** Обязателен вместе с onRemove — без RU-заглушки по умолчанию (Stage 5 аудит,
       * тот же принцип, что у QuantityStepper: локализуемый aria-label — забота вызывающей фичи). */
      removeLabel: string;
    });

export function AttachmentChip({
  name,
  mimeType,
  sizeBytes,
  onRemove,
  removeLabel,
}: AttachmentChipProps) {
  const kind = detectAttachmentKind(name, mimeType);
  const Icon = KIND_ICON[kind];

  return (
    <div className="flex items-center gap-3 rounded-xl border border-line bg-surface px-3.5 py-2.5 shadow-card">
      <div className="grid h-10 w-10 flex-none place-items-center rounded-md border border-line bg-surface-2 text-muted">
        <Icon size={18} aria-hidden="true" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-body-sm font-semibold text-ink">{name}</div>
        <div className="font-mono text-micro uppercase text-muted">
          {formatFileSize(sizeBytes)} · {kind}
        </div>
      </div>
      {onRemove && (
        <IconButton aria-label={removeLabel} onClick={onRemove}>
          <X size={16} aria-hidden="true" />
        </IconButton>
      )}
    </div>
  );
}
