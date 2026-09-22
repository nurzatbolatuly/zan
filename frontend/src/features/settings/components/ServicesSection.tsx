import { Plus } from "lucide-react";
import { Button, Card } from "@/shared/ui";
import type { Service, ServiceFormResult, ServiceId } from "../types";
import { ServiceRow } from "./ServiceRow";
import { ServiceCreateModal } from "./ServiceCreateModal";

interface ServicesSectionProps {
  title: string;
  subtitle: string;
  typeFieldLabel: string;
  nameFieldLabel: string;
  priceFieldLabel: string;
  addLabel: string;
  deleteLabel: string;
  services: Service[];
  onChange: (
    id: ServiceId,
    patch: Partial<Pick<Service, "name" | "unitPriceTenge">>,
  ) => void;
  onCommit: (service: Service) => void;
  onDelete: (service: Service) => void;
  isCreateModalOpen: boolean;
  createModalKey: string;
  modalTitle: string;
  modalLabels: {
    save: string;
    cancel: string;
    typeRequiredError: string;
    nameRequiredError: string;
    priceRequiredError: string;
  };
  onOpenCreate: () => void;
  onCloseCreate: () => void;
  onCreate: (values: ServiceFormResult) => void;
}

/** Settings → Тарифы → список услуг (PLAN.md §5 Stage 4b + create/delete на моках). */
export function ServicesSection({
  title,
  subtitle,
  typeFieldLabel,
  nameFieldLabel,
  priceFieldLabel,
  addLabel,
  deleteLabel,
  services,
  onChange,
  onCommit,
  onDelete,
  isCreateModalOpen,
  createModalKey,
  modalTitle,
  modalLabels,
  onOpenCreate,
  onCloseCreate,
  onCreate,
}: ServicesSectionProps) {
  return (
    <Card>
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-h3 text-ink">{title}</h2>
          <p className="text-caption text-muted">{subtitle}</p>
        </div>
        <Button variant="secondary" size="sm" onClick={onOpenCreate}>
          <Plus size={16} aria-hidden="true" />
          {addLabel}
        </Button>
      </div>
      <div className="flex flex-col gap-2.5">
        {services.map((service) => (
          <ServiceRow
            key={service.id}
            service={service}
            typeFieldLabel={typeFieldLabel}
            nameFieldLabel={nameFieldLabel}
            priceFieldLabel={priceFieldLabel}
            deleteLabel={deleteLabel}
            onChange={(patch) => onChange(service.id, patch)}
            onCommit={() => onCommit(service)}
            onDelete={() => onDelete(service)}
          />
        ))}
      </div>

      <ServiceCreateModal
        key={createModalKey}
        open={isCreateModalOpen}
        title={modalTitle}
        labels={{
          typeField: typeFieldLabel,
          nameField: nameFieldLabel,
          priceField: priceFieldLabel,
          ...modalLabels,
        }}
        onSave={onCreate}
        onClose={onCloseCreate}
      />
    </Card>
  );
}
