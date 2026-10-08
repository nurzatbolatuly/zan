import { Upload } from "lucide-react";
import { Badge, Button, Card, EmptyState } from "@/shared/ui";
import type { TemplatesDictionary } from "../locales";
import type { DocumentTemplate, TemplateGroup } from "../types";
import { TemplateCard } from "./TemplateCard";

interface TemplatesSectionProps {
  labels: TemplatesDictionary;
  groups: TemplateGroup[];
  hasTemplates: boolean;
  /** Без единого типа в справочнике шаблон не к чему отнести — загрузка недоступна. */
  canUpload: boolean;
  onUpload: () => void;
  onView: (template: DocumentTemplate) => void;
  onEdit: (template: DocumentTemplate) => void;
  onDelete: (template: DocumentTemplate) => void;
}

/** Settings → Шаблоны → шаблоны, сгруппированные по типу документа. */
export function TemplatesSection({
  labels,
  groups,
  hasTemplates,
  canUpload,
  onUpload,
  onView,
  onEdit,
  onDelete,
}: TemplatesSectionProps) {
  return (
    <Card>
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-h3 text-ink">{labels.templatesTitle}</h2>
          <p className="text-caption text-muted">{labels.templatesSub}</p>
        </div>
        <Button size="sm" onClick={onUpload} disabled={!canUpload}>
          <Upload size={16} aria-hidden="true" />
          {labels.uploadTemplate}
        </Button>
      </div>

      {!canUpload && <p className="mb-3 text-caption text-muted">{labels.noTypesHint}</p>}

      {hasTemplates ? (
        <div className="flex flex-col gap-5">
          {groups.map(({ type, templates }) => (
            <section key={type.id} aria-label={type.name}>
              <div className="mb-2 flex items-center gap-2">
                <h3 className="text-body-sm font-semibold text-ink">{type.name}</h3>
                <Badge>{templates.length}</Badge>
              </div>
              <div className="flex flex-col gap-2">
                {templates.map((template) => (
                  <TemplateCard
                    key={template.id}
                    template={template}
                    labels={labels}
                    onView={() => onView(template)}
                    onEdit={() => onEdit(template)}
                    onDelete={() => onDelete(template)}
                  />
                ))}
              </div>
            </section>
          ))}
        </div>
      ) : (
        <EmptyState title={labels.emptyTitle} description={labels.emptyDescription} />
      )}
    </Card>
  );
}
