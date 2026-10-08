import { Button, EmptyState, Skeleton } from "@/shared/ui";
import { useLangStore } from "@/shared/stores/useLangStore";
import { settingsDictionary } from "../locales";
import { useAdminDocumentTypes } from "../useAdminDocumentTypes";
import { useAdminDocumentTemplates } from "../useAdminDocumentTemplates";
import { TemplatesSection } from "./TemplatesSection";
import { DocumentTypesSection } from "./DocumentTypesSection";
import { TemplateEditModal } from "./TemplateEditModal";
import { TemplatePreviewModal } from "./TemplatePreviewModal";
import { DocumentTypeModal } from "./DocumentTypeModal";

/**
 * Settings → Шаблоны: шаблоны документов (PDF/DOCX, просмотр как PDF) и
 * справочник их типов — `/admin/document-templates`, `/admin/document-types`.
 */
export function TemplatesTab() {
  const lang = useLangStore((state) => state.lang);
  const { common, templates: t } = settingsDictionary[lang];
  const types = useAdminDocumentTypes();
  const templates = useAdminDocumentTemplates(types.types);

  if (types.isError || templates.isError) {
    return (
      <EmptyState
        title={common.loadError}
        action={
          <Button
            onClick={() => {
              types.retry();
              templates.retry();
            }}
          >
            {common.retry}
          </Button>
        }
      />
    );
  }

  if (types.isLoading || templates.isLoading) {
    return (
      <div className="flex flex-col gap-3.5">
        <Skeleton className="h-56" />
        <Skeleton className="h-40" />
      </div>
    );
  }

  const { templateModal } = templates;
  const { typeModal } = types;

  return (
    <div className="flex flex-col gap-3.5">
      <TemplatesSection
        labels={t}
        groups={templates.groups}
        hasTemplates={templates.hasTemplates}
        canUpload={types.types.length > 0}
        onUpload={templates.openNewTemplate}
        onView={templates.openPreview}
        onEdit={templates.openEditTemplate}
        onDelete={templates.requestDeleteTemplate}
      />

      <DocumentTypesSection
        labels={t}
        types={types.types}
        countsByType={templates.countsByType}
        onAdd={types.openNewType}
        onRename={types.openRenameType}
        onDelete={types.requestDeleteType}
      />

      <TemplateEditModal
        key={`template-${templates.templateModalKey}`}
        open={templateModal.mode !== "closed"}
        types={types.types}
        initialTemplate={templateModal.mode === "edit" ? templateModal.template : null}
        labels={t}
        saveLabel={common.save}
        cancelLabel={common.cancel}
        isPending={templates.isTemplateModalPending}
        onSave={templates.saveTemplate}
        onClose={templates.closeTemplateModal}
      />

      <DocumentTypeModal
        key={`type-${types.typeModalKey}`}
        open={typeModal.mode !== "closed"}
        initialType={typeModal.mode === "rename" ? typeModal.type : null}
        labels={t}
        saveLabel={common.save}
        cancelLabel={common.cancel}
        isPending={types.isTypeModalPending}
        onSave={types.saveType}
        onClose={types.closeTypeModal}
      />

      <TemplatePreviewModal
        template={templates.previewTemplate}
        labels={t}
        onClose={templates.closePreview}
      />
    </div>
  );
}
