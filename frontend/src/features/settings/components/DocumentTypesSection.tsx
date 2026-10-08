import { Pencil, Plus, Trash2 } from "lucide-react";
import { Button, Card, IconButton } from "@/shared/ui";
import type { TemplatesDictionary } from "../locales";
import type { DocumentType } from "../types";

interface DocumentTypesSectionProps {
  labels: TemplatesDictionary;
  types: DocumentType[];
  countsByType: Map<string, number>;
  onAdd: () => void;
  onRename: (type: DocumentType) => void;
  onDelete: (type: DocumentType) => void;
}

/** Settings → Шаблоны → справочник типов документов с числом шаблонов у каждого. */
export function DocumentTypesSection({
  labels,
  types,
  countsByType,
  onAdd,
  onRename,
  onDelete,
}: DocumentTypesSectionProps) {
  return (
    <Card>
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-h3 text-ink">{labels.typesTitle}</h2>
          <p className="text-caption text-muted">{labels.typesSub}</p>
        </div>
        <Button variant="secondary" size="sm" onClick={onAdd}>
          <Plus size={16} aria-hidden="true" />
          {labels.addType}
        </Button>
      </div>

      <ul className="flex flex-col divide-y divide-line">
        {types.map((type) => (
          <li key={type.id} className="flex items-center gap-2 py-1">
            <div className="min-w-0 flex-1">
              <div className="truncate text-body-sm text-ink">{type.name}</div>
              <div className="font-mono text-micro uppercase text-muted">
                {labels.templatesCount(countsByType.get(type.id) ?? 0)}
              </div>
            </div>
            <IconButton
              aria-label={`${labels.renameTypeTitle}: ${type.name}`}
              variant="ghost"
              onClick={() => onRename(type)}
            >
              <Pencil size={14} aria-hidden="true" />
            </IconButton>
            <IconButton
              aria-label={`${labels.deleteAction}: ${type.name}`}
              variant="ghost-danger"
              onClick={() => onDelete(type)}
            >
              <Trash2 size={14} aria-hidden="true" />
            </IconButton>
          </li>
        ))}
      </ul>
    </Card>
  );
}
