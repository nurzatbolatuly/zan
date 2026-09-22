import { useState } from "react";
import type { FormEvent } from "react";
import { Modal, Input, Button } from "@/shared/ui";
import type { ServiceFormResult } from "../types";

interface ServiceCreateModalLabels {
  typeField: string;
  nameField: string;
  priceField: string;
  save: string;
  cancel: string;
  typeRequiredError: string;
  nameRequiredError: string;
  priceRequiredError: string;
}

interface ServiceCreateModalProps {
  open: boolean;
  title: string;
  labels: ServiceCreateModalLabels;
  onSave: (values: ServiceFormResult) => void;
  onClose: () => void;
}

interface FormErrors {
  typeLabel?: string;
  name?: string;
  unitPriceTenge?: string;
}

/**
 * Модалка создания услуги (Settings → Тарифы). Только create, без edit — у
 * существующих услуг название/цена правятся инлайн (`ServiceRow`), а тип
 * после создания не меняется. Локальный `useState` вместо react-hook-form/zod
 * (как в `BundleEditModal`) — три плоских поля без live-пересчёта суммы, RHF
 * был бы избыточен. Родитель монтирует компонент с `key` (см. `TariffsTab.tsx`
 * / `useAdminTariffs.ts#serviceModalKey`), поэтому форма всегда стартует с
 * чистого состояния — без ручного reset() при каждом открытии/отмене.
 */
export function ServiceCreateModal({
  open,
  title,
  labels,
  onSave,
  onClose,
}: ServiceCreateModalProps) {
  const [typeLabel, setTypeLabel] = useState("");
  const [name, setName] = useState("");
  const [unitPriceTenge, setUnitPriceTenge] = useState("");
  const [errors, setErrors] = useState<FormErrors>({});

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    const price = Number(unitPriceTenge);
    const nextErrors: FormErrors = {};
    if (!typeLabel.trim()) nextErrors.typeLabel = labels.typeRequiredError;
    if (!name.trim()) nextErrors.name = labels.nameRequiredError;
    if (!(price > 0)) nextErrors.unitPriceTenge = labels.priceRequiredError;
    if (Object.keys(nextErrors).length > 0) {
      setErrors(nextErrors);
      return;
    }
    onSave({ typeLabel: typeLabel.trim(), name: name.trim(), unitPriceTenge: price });
  }

  return (
    <Modal open={open} onClose={onClose} ariaLabel={title}>
      <div className="mb-4 text-h3 text-ink">{title}</div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-3.5">
        <div>
          <Input
            label={labels.typeField}
            value={typeLabel}
            onChange={(event) => setTypeLabel(event.target.value)}
          />
          {errors.typeLabel && (
            <p className="mt-1 text-caption text-danger">{errors.typeLabel}</p>
          )}
        </div>

        <div>
          <Input
            label={labels.nameField}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
          {errors.name && <p className="mt-1 text-caption text-danger">{errors.name}</p>}
        </div>

        <div>
          <Input
            type="number"
            inputMode="numeric"
            min={0}
            label={labels.priceField}
            value={unitPriceTenge}
            onChange={(event) => setUnitPriceTenge(event.target.value)}
          />
          {errors.unitPriceTenge && (
            <p className="mt-1 text-caption text-danger">{errors.unitPriceTenge}</p>
          )}
        </div>

        <div className="flex flex-col gap-2">
          <Button type="submit">{labels.save}</Button>
          <Button type="button" variant="ghost" onClick={onClose}>
            {labels.cancel}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
