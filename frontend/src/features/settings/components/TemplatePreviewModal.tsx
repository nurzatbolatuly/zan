import { ExternalLink, X } from "lucide-react";
import { IconButton, Modal } from "@/shared/ui";
import type { TemplatesDictionary } from "../locales";
import type { DocumentTemplate } from "../types";

interface TemplatePreviewModalProps {
  template: DocumentTemplate | null;
  labels: TemplatesDictionary;
  onClose: () => void;
}

/**
 * Просмотр шаблона как PDF во встроенном просмотрщике браузера — только
 * чтение, файл не меняется (у DOCX показывается PDF-копия, построенная
 * бэком при загрузке).
 */
export function TemplatePreviewModal({
  template,
  labels,
  onClose,
}: TemplatePreviewModalProps) {
  return (
    <Modal
      open={template !== null}
      onClose={onClose}
      ariaLabel={template?.title ?? ""}
      size="lg"
    >
      {template && (
        <>
          <div className="mb-3 flex items-center gap-2">
            <div className="min-w-0 flex-1 truncate text-h3 text-ink">
              {template.title}
            </div>
            <a
              href={template.previewUrl}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={labels.openInNewTab}
              title={labels.openInNewTab}
              className="grid h-11 w-11 flex-none place-items-center rounded-md text-muted transition-colors hover:bg-surface-2 hover:text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
            >
              <ExternalLink size={16} aria-hidden="true" />
            </a>
            <IconButton aria-label={labels.close} variant="ghost" onClick={onClose}>
              <X size={16} aria-hidden="true" />
            </IconButton>
          </div>
          <iframe
            src={template.previewUrl}
            title={template.title}
            className="h-[70vh] w-full rounded-lg border border-line bg-surface-2"
          />
        </>
      )}
    </Modal>
  );
}
