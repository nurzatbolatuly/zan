import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useConfirmModalStore } from "@/shared/stores/useConfirmModalStore";
import { useToast } from "@/shared/ui";
import { logger } from "@/shared/lib/logger";
import { api } from "@/shared/lib/api";
import { describeApiError } from "@/shared/lib/apiErrorMessages";
import { countTemplatesByType, groupTemplatesByType } from "./templateFiles";
import { settingsDictionary } from "./locales";
import type { DocumentTemplate, DocumentType, TemplateFormResult } from "./types";
import type { DocumentTemplateDto } from "@/shared/types/api";

const DOCUMENT_TEMPLATES_QUERY_KEY = ["admin", "document-templates"] as const;

export type TemplateModalState =
  { mode: "closed" } | { mode: "create" } | { mode: "edit"; template: DocumentTemplate };

function mapTemplate(dto: DocumentTemplateDto): DocumentTemplate {
  return {
    id: dto.id,
    typeId: dto.document_type_id,
    title: dto.title,
    originalName: dto.original_name,
    fileKind: dto.mime_type === "application/pdf" ? "pdf" : "docx",
    sizeBytes: dto.size_bytes,
    previewUrl: dto.preview_url,
    updatedAt: dto.updated_at,
  };
}

/** multipart-тело POST/PUT `/admin/document-templates` (`openapi.yaml`). */
function toFormData(values: TemplateFormResult): FormData {
  const body = new FormData();
  body.append("document_type_id", values.typeId);
  body.append("title", values.title);
  if (values.file) body.append("file", values.file);
  return body;
}

/**
 * Settings → Шаблоны → шаблоны документов (`/admin/document-templates`,
 * требует `X-Admin-Token`). `types` — справочник из `useAdminDocumentTypes`:
 * по нему шаблоны раскладываются на группы. Просмотр — PDF-версия по
 * `previewUrl` (у DOCX её строит бэк при загрузке), без редактирования.
 */
export function useAdminDocumentTemplates(types: DocumentType[]) {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang];
  const toast = useToast();
  const openConfirm = useConfirmModalStore((state) => state.open);
  const queryClient = useQueryClient();

  const templatesQuery = useQuery({
    queryKey: DOCUMENT_TEMPLATES_QUERY_KEY,
    queryFn: () =>
      api.get<DocumentTemplateDto[]>("/admin/document-templates", { admin: true }),
  });
  const templates = useMemo(
    () => (templatesQuery.data ?? []).map(mapTemplate),
    [templatesQuery.data],
  );
  const groups = useMemo(
    () => groupTemplatesByType(templates, types),
    [templates, types],
  );
  const countsByType = useMemo(() => countTemplatesByType(templates), [templates]);

  const [templateModal, setTemplateModal] = useState<TemplateModalState>({
    mode: "closed",
  });
  // См. useAdminDocumentTypes#typeModalKey — форма пересобирается при каждом открытии.
  const [templateModalKey, setTemplateModalKey] = useState(0);
  const [isTemplateModalPending, setTemplateModalPending] = useState(false);
  const [previewTemplate, setPreviewTemplate] = useState<DocumentTemplate | null>(null);

  function openModal(state: TemplateModalState) {
    setTemplateModalKey((key) => key + 1);
    setTemplateModal(state);
  }

  function closeTemplateModal() {
    setTemplateModal({ mode: "closed" });
  }

  async function saveTemplate(values: TemplateFormResult) {
    setTemplateModalPending(true);
    try {
      if (templateModal.mode === "create") {
        await api.post("/admin/document-templates", toFormData(values), { admin: true });
        logger.info({ scope: "settings.templates", event: "template_created" });
      } else if (templateModal.mode === "edit") {
        await api.put(
          `/admin/document-templates/${templateModal.template.id}`,
          toFormData(values),
          { admin: true },
        );
        logger.info({
          scope: "settings.templates",
          event: "template_updated",
          data: {
            templateId: templateModal.template.id,
            fileReplaced: values.file !== null,
          },
        });
      }
      await queryClient.invalidateQueries({ queryKey: DOCUMENT_TEMPLATES_QUERY_KEY });
      toast(t.templates.templateSavedToast, "success");
      closeTemplateModal();
    } catch (error) {
      logger.error({ scope: "settings.templates", event: "template_save_failed", error });
      toast(describeApiError(error, lang), "error");
    } finally {
      setTemplateModalPending(false);
    }
  }

  function requestDeleteTemplate(template: DocumentTemplate) {
    logger.info({
      scope: "settings.templates",
      event: "template_delete_requested",
      data: { templateId: template.id },
    });
    openConfirm({
      title: t.templates.templateDeleteConfirmTitle,
      message: t.templates.templateDeleteConfirmMessage,
      confirmLabel: t.templates.deleteAction,
      cancelLabel: t.common.cancel,
      onConfirm: async () => {
        await api.delete(`/admin/document-templates/${template.id}`, { admin: true });
        logger.info({
          scope: "settings.templates",
          event: "template_deleted",
          data: { templateId: template.id },
        });
        await queryClient.invalidateQueries({ queryKey: DOCUMENT_TEMPLATES_QUERY_KEY });
        toast(t.templates.templateDeletedToast, "success");
      },
    });
  }

  function openPreview(template: DocumentTemplate) {
    logger.info({
      scope: "settings.templates",
      event: "template_preview_opened",
      data: { templateId: template.id },
    });
    setPreviewTemplate(template);
  }

  return {
    hasTemplates: templates.length > 0,
    groups,
    countsByType,
    isLoading: templatesQuery.isLoading,
    isError: templatesQuery.isError,
    retry: () => void templatesQuery.refetch(),
    templateModal,
    templateModalKey,
    isTemplateModalPending,
    openNewTemplate: () => openModal({ mode: "create" }),
    openEditTemplate: (template: DocumentTemplate) =>
      openModal({ mode: "edit", template }),
    closeTemplateModal,
    saveTemplate,
    requestDeleteTemplate,
    previewTemplate,
    openPreview,
    closePreview: () => setPreviewTemplate(null),
  };
}
