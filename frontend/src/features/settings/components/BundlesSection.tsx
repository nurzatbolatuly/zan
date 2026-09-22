import { Plus } from "lucide-react";
import { Button, Card } from "@/shared/ui";
import type { BundlePrice } from "@/shared/lib/tariffPricing";
import type { Bundle, BundleFormResult, Service } from "../types";
import { BundleCard } from "./BundleCard";
import { BundleEditModal } from "./BundleEditModal";

type BundleModalState =
  { mode: "closed" } | { mode: "create" } | { mode: "edit"; bundle: Bundle };

interface BundlesSectionProps {
  title: string;
  subtitle: string;
  addLabel: string;
  editLabel: string;
  deleteLabel: string;
  bundlesView: Array<{
    bundle: Bundle;
    includesItems: string[];
    discountLabel: string;
    price: BundlePrice;
  }>;
  services: Service[];
  bundleModal: BundleModalState;
  bundleModalKey: string;
  modalTitles: { create: string; edit: string };
  modalLabels: {
    fName: string;
    fDiscount: string;
    total: string;
    save: string;
    cancel: string;
    nameRequiredError: string;
    atLeastOneItemError: string;
    decreaseQuantityLabel: (serviceName: string) => string;
    increaseQuantityLabel: (serviceName: string) => string;
  };
  onOpenNew: () => void;
  onOpenEdit: (bundle: Bundle) => void;
  onDelete: (bundle: Bundle) => void;
  onSave: (values: BundleFormResult) => void;
  onCloseModal: () => void;
}

/** Settings → Тарифы → сетка тарифов + редактор (PLAN.md §5 Stage 4b). */
export function BundlesSection({
  title,
  subtitle,
  addLabel,
  editLabel,
  deleteLabel,
  bundlesView,
  services,
  bundleModal,
  bundleModalKey,
  modalTitles,
  modalLabels,
  onOpenNew,
  onOpenEdit,
  onDelete,
  onSave,
  onCloseModal,
}: BundlesSectionProps) {
  return (
    <Card>
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-h3 text-ink">{title}</h2>
          <p className="text-caption text-muted">{subtitle}</p>
        </div>
        <Button variant="secondary" size="sm" onClick={onOpenNew}>
          <Plus size={16} aria-hidden="true" />
          {addLabel}
        </Button>
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {bundlesView.map(({ bundle, includesItems, discountLabel, price }) => (
          <BundleCard
            key={bundle.id}
            bundle={bundle}
            includesItems={includesItems}
            discountLabel={discountLabel}
            price={price}
            editLabel={editLabel}
            deleteLabel={deleteLabel}
            onEdit={() => onOpenEdit(bundle)}
            onDelete={() => onDelete(bundle)}
          />
        ))}
      </div>

      {/* `key` пересобирает форму модалки при каждом открытии (см. useAdminTariffs.ts#bundleModalKey). */}
      <BundleEditModal
        key={bundleModalKey}
        open={bundleModal.mode !== "closed"}
        title={bundleModal.mode === "edit" ? modalTitles.edit : modalTitles.create}
        services={services}
        initialBundle={bundleModal.mode === "edit" ? bundleModal.bundle : null}
        labels={modalLabels}
        onSave={onSave}
        onClose={onCloseModal}
      />
    </Card>
  );
}
