import { Controller, type Control } from "react-hook-form";
import { Modal, Button, Input, QuantityStepper } from "@/shared/ui";
import { formatTenge } from "@/shared/lib/format";
import { useBundleForm } from "../useBundleForm";
import type { BundleFormValues } from "../useBundleForm";
import type { Bundle, BundleFormResult, Service } from "../types";

interface BundleEditModalLabels {
  fName: string;
  fDiscount: string;
  total: string;
  save: string;
  cancel: string;
  nameRequiredError: string;
  atLeastOneItemError: string;
  decreaseQuantityLabel: (serviceName: string) => string;
  increaseQuantityLabel: (serviceName: string) => string;
}

interface BundleEditModalProps {
  open: boolean;
  title: string;
  services: Service[];
  initialBundle: Bundle | null;
  labels: BundleEditModalLabels;
  onSave: (values: BundleFormResult) => void;
  onClose: () => void;
}

// Обёртка над QuantityStepper, подключающая его к RHF-контролу — Controller-паттерн,
// приватный для этой модалки (не самостоятельная переиспользуемая единица UI).
// Тот же набор пропов (decreaseLabel/increaseLabel/inputLabel), что и у Stage 3
// (features/tariffs/components/CustomOrderModal.tsx) — второй реальный
// потребитель QuantityStepper после того, как Stage 3 заменил его прежний
// единственный `label` на явную локализацию каждой aria-подписи.
function QuantityStepperField({
  control,
  serviceId,
  name,
  decreaseLabel,
  increaseLabel,
}: {
  control: Control<BundleFormValues>;
  serviceId: string;
  name: string;
  decreaseLabel: string;
  increaseLabel: string;
}) {
  return (
    <Controller
      control={control}
      name={`quantities.${serviceId}`}
      render={({ field }) => (
        <QuantityStepper
          value={field.value}
          max={20}
          onChange={field.onChange}
          decreaseLabel={decreaseLabel}
          increaseLabel={increaseLabel}
          inputLabel={name}
        />
      )}
    />
  );
}

/**
 * M2 — редактор тарифа (PLAN.md §1, §5 Stage 4b). Родитель монтирует этот
 * компонент с `key`, завязанным на редактируемый тариф (см. `TariffsTab.tsx`),
 * поэтому `useBundleForm` всегда стартует с чистыми `defaultValues` — без
 * ручного `reset()` при смене тарифа.
 */
export function BundleEditModal({
  open,
  title,
  services,
  initialBundle,
  labels,
  onSave,
  onClose,
}: BundleEditModalProps) {
  const { register, formState, price, submitForm, control } = useBundleForm({
    services,
    initialBundle,
    nameRequiredError: labels.nameRequiredError,
    atLeastOneItemError: labels.atLeastOneItemError,
    onSave,
  });
  // react-hook-form типизирует ошибку record-поля (`quantities: Record<string, number>`)
  // так, что TS видит `.message` как `FieldError`, а не `string` (индексная сигнатура
  // рекорда конфликтует с полем `message` самого FieldError) — приводим явно, ошибка
  // здесь всегда объектного уровня (zod `.refine({ path: ["quantities"] })`), не по конкретному serviceId.
  const quantitiesError = formState.errors.quantities as { message?: string } | undefined;

  return (
    <Modal open={open} onClose={onClose} ariaLabel={title}>
      <div className="mb-4 text-h3 text-ink">{title}</div>

      <form onSubmit={submitForm} className="flex flex-col gap-3.5">
        <Input label={labels.fName} {...register("name")} />
        {formState.errors.name && (
          <p className="-mt-2.5 text-caption text-danger">
            {formState.errors.name.message}
          </p>
        )}

        <Input
          type="number"
          inputMode="numeric"
          min={0}
          max={90}
          label={labels.fDiscount}
          {...register("discountPercent", { valueAsNumber: true })}
        />

        <div className="flex flex-col gap-2">
          {services.map((service) => (
            <div
              key={service.id}
              className="flex items-center gap-2.5 rounded-lg border border-line px-3 py-2.5"
            >
              <div className="min-w-0 flex-1 truncate text-body-sm text-ink">
                {service.name}
              </div>
              <QuantityStepperField
                control={control}
                serviceId={service.id}
                name={service.name}
                decreaseLabel={labels.decreaseQuantityLabel(service.name)}
                increaseLabel={labels.increaseQuantityLabel(service.name)}
              />
            </div>
          ))}
        </div>
        {quantitiesError?.message && (
          <p className="text-caption text-danger">{quantitiesError.message}</p>
        )}

        <div className="flex items-center justify-between rounded-xl border border-line bg-bg px-3.5 py-3">
          <span className="text-body-sm text-ink">{labels.total}</span>
          <strong className="text-body-lg text-ink">
            {formatTenge(price.totalTenge)}
          </strong>
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
