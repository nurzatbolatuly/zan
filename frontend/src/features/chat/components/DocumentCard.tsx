import { Download, Pencil } from "lucide-react";
import { Button } from "@/shared/ui/Button";
import { useToast } from "@/shared/ui/toast/useToast";
import { logger } from "@/shared/lib/logger";
import type { ChatDocument } from "../types";
import type { ChatDictionary } from "../locales";

interface DocumentCardProps {
  document: ChatDocument;
  t: ChatDictionary;
}

/** Карточка готового документа (Zan.dc.html:138-154) — скачивание/правка появятся в Stage 6. */
export function DocumentCard({ document, t }: DocumentCardProps) {
  const toast = useToast();

  function handleUnavailableAction(action: "download_pdf" | "download_docx" | "edit") {
    logger.info({ scope: "chat.document", event: "action_clicked", data: { action } });
    toast(t.documentActionUnavailable, "info");
  }

  return (
    <div className="rounded-xl border border-accent bg-surface p-4 shadow-card">
      <div className="flex flex-wrap items-start gap-3.5">
        <div className="h-[58px] w-[46px] flex-none rounded-md border border-line bg-surface-2" />
        <div className="min-w-[180px] flex-1">
          <div className="mb-1 text-h3 text-ink">{document.title}</div>
          <p className="text-caption text-muted">{document.description}</p>
        </div>
      </div>
      <div className="mt-4 flex flex-wrap gap-2">
        <Button size="sm" onClick={() => handleUnavailableAction("download_pdf")}>
          <Download size={16} aria-hidden="true" />
          {t.downloadPdf}
        </Button>
        <Button
          variant="secondary"
          size="sm"
          onClick={() => handleUnavailableAction("download_docx")}
        >
          <Download size={16} aria-hidden="true" />
          {t.downloadDocx}
        </Button>
        <Button
          variant="secondary"
          size="sm"
          onClick={() => handleUnavailableAction("edit")}
        >
          <Pencil size={16} aria-hidden="true" />
          {t.editDocument}
        </Button>
      </div>
    </div>
  );
}
