import { useRef } from "react";
import { FileText, FileType2, Paperclip } from "lucide-react";
import { Button, Input, Modal, Select } from "@/shared/ui";
import { formatFileSize } from "@/shared/lib/format";
import { TEMPLATE_FILE_ACCEPT, TEMPLATE_TITLE_MAX_LENGTH } from "../templateFiles";
import { useTemplateForm } from "../useTemplateForm";
import type { TemplatesDictionary } from "../locales";
import type { DocumentTemplate, DocumentType, TemplateFormResult } from "../types";

interface TemplateEditModalProps {
  open: boolean;
  types: DocumentType[];
  /** `null` — новый шаблон (файл обязателен), иначе изменение (файл — замена). */
  initialTemplate: DocumentTemplate | null;
  labels: TemplatesDictionary;
  saveLabel: string;
  cancelLabel: string;
  isPending: boolean;
  onSave: (values: TemplateFormResult) => void;
  onClose: () => void;
}

/**
 * Загрузка/изменение шаблона: название, тип документа, файл. Родитель
 * монтирует модалку с новым `key` на каждое открытие — форма всегда стартует
 * с чистых значений (см. useAdminDocumentTemplates#templateModalKey).
 */
export function TemplateEditModal({
  open,
  types,
  initialTemplate,
  labels,
  saveLabel,
  cancelLabel,
  isPending,
  onSave,
  onClose,
}: TemplateEditModalProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const {
    register,
    errors,
    selectedFile,
    selectFile,
    maxSizeLabel,
    isCreate,
    submitForm,
  } = useTemplateForm({ initialTemplate, messages: labels, onSave });
  const title = isCreate ? labels.newTemplateTitle : labels.editTemplateTitle;

  // Показывается выбранный файл, а при изменении без замены — текущий.
  const shownFile = selectedFile
    ? { name: selectedFile.name, sizeBytes: selectedFile.size }
    : initialTemplate && {
        name: initialTemplate.originalName,
        sizeBytes: initialTemplate.sizeBytes,
      };
  const FileIcon = shownFile?.name.toLowerCase().endsWith(".pdf") ? FileText : FileType2;

  return (
    <Modal open={open} onClose={onClose} ariaLabel={title}>
      <div className="mb-4 text-h3 text-ink">{title}</div>

      <form onSubmit={submitForm} className="flex flex-col gap-3.5">
        <div>
          <span className="mb-1 block text-body-sm font-semibold text-ink">
            {labels.fFile}
          </span>
          {shownFile && (
            <div className="mb-2 flex items-center gap-3 rounded-lg border border-line px-3 py-2.5">
              <FileIcon size={18} className="flex-none text-muted" aria-hidden="true" />
              <div className="min-w-0 flex-1">
                <div className="truncate text-body-sm text-ink">{shownFile.name}</div>
                <div className="font-mono text-micro uppercase text-muted">
                  {formatFileSize(shownFile.sizeBytes)}
                </div>
              </div>
            </div>
          )}
          <Button
            variant="secondary"
            size="sm"
            onClick={() => fileInputRef.current?.click()}
            disabled={isPending}
          >
            <Paperclip size={16} aria-hidden="true" />
            {shownFile ? labels.replaceFile : labels.chooseFile}
          </Button>
          <input
            ref={fileInputRef}
            type="file"
            accept={TEMPLATE_FILE_ACCEPT}
            aria-label={labels.fFile}
            className="hidden"
            onChange={(event) => {
              selectFile(event.target.files?.[0] ?? null);
              // Сброс — повторный выбор того же файла тоже должен вызвать onChange.
              event.target.value = "";
            }}
          />
          <span className="mt-1 block text-caption text-muted">
            {labels.fileHint(maxSizeLabel)}
          </span>
          {errors.file && (
            <p className="mt-1 text-caption text-danger">{errors.file.message}</p>
          )}
        </div>

        <div>
          <Input
            label={labels.fTitle}
            maxLength={TEMPLATE_TITLE_MAX_LENGTH}
            {...register("title")}
          />
          {errors.title && (
            <p className="mt-1 text-caption text-danger">{errors.title.message}</p>
          )}
        </div>

        <div>
          <Select label={labels.fType} {...register("typeId")}>
            <option value="" disabled>
              {labels.fTypePlaceholder}
            </option>
            {types.map((type) => (
              <option key={type.id} value={type.id}>
                {type.name}
              </option>
            ))}
          </Select>
          {errors.typeId && (
            <p className="mt-1 text-caption text-danger">{errors.typeId.message}</p>
          )}
        </div>

        {isPending && <p className="text-caption text-muted">{labels.savingNote}</p>}

        <div className="flex flex-col gap-2">
          <Button type="submit" disabled={isPending}>
            {isPending ? "…" : saveLabel}
          </Button>
          <Button type="button" variant="ghost" onClick={onClose} disabled={isPending}>
            {cancelLabel}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
