import { Eye, FileText, FileType2, Pencil, Trash2 } from "lucide-react";
import { IconButton } from "@/shared/ui";
import { formatDate, formatFileSize } from "@/shared/lib/format";
import type { TemplatesDictionary } from "../locales";
import type { DocumentTemplate, TemplateFileKind } from "../types";

const KIND_ICON: Record<TemplateFileKind, typeof FileText> = {
  pdf: FileText,
  docx: FileType2,
};

interface TemplateCardProps {
  template: DocumentTemplate;
  labels: TemplatesDictionary;
  onView: () => void;
  onEdit: () => void;
  onDelete: () => void;
}

/** Строка шаблона: название открывает просмотр, справа — просмотр/изменение/удаление. */
export function TemplateCard({
  template,
  labels,
  onView,
  onEdit,
  onDelete,
}: TemplateCardProps) {
  const Icon = KIND_ICON[template.fileKind];

  return (
    <div className="flex items-center gap-3 rounded-lg border border-line px-3 py-2.5">
      <div className="grid h-10 w-10 flex-none place-items-center rounded-md border border-line bg-surface-2 text-muted">
        <Icon size={18} aria-hidden="true" />
      </div>
      <button
        type="button"
        onClick={onView}
        className="min-w-0 flex-1 rounded-md text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
      >
        <div className="truncate text-body-sm font-semibold text-ink">
          {template.title}
        </div>
        <div className="truncate font-mono text-micro text-muted">
          {template.originalName} · {formatFileSize(template.sizeBytes)} ·{" "}
          {formatDate(template.updatedAt)}
        </div>
      </button>
      <div className="-mr-1.5 flex flex-none items-center gap-0.5">
        <IconButton
          aria-label={`${labels.viewAction}: ${template.title}`}
          variant="ghost"
          onClick={onView}
        >
          <Eye size={14} aria-hidden="true" />
        </IconButton>
        <IconButton
          aria-label={`${labels.editAction}: ${template.title}`}
          variant="ghost"
          onClick={onEdit}
        >
          <Pencil size={14} aria-hidden="true" />
        </IconButton>
        <IconButton
          aria-label={`${labels.deleteAction}: ${template.title}`}
          variant="ghost-danger"
          onClick={onDelete}
        >
          <Trash2 size={14} aria-hidden="true" />
        </IconButton>
      </div>
    </div>
  );
}
