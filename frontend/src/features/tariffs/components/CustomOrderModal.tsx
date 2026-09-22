import { Button, Modal, QuantityStepper } from "@/shared/ui";
import { formatTenge } from "@/shared/lib/format";
import type { TariffsDictionary } from "../locales";
import type { BuiltInServiceId, CustomOrderQuantities } from "../types";

interface CustomOrderModalProps {
  open: boolean;
  onClose: () => void;
  dictionary: TariffsDictionary;
  serviceIds: BuiltInServiceId[];
  quantities: CustomOrderQuantities;
  onQuantityChange: (serviceId: BuiltInServiceId, qty: number) => void;
  totalTenge: number;
  canConfirm: boolean;
  onConfirm: () => void;
  maxQty: number;
  unitPrices: Record<BuiltInServiceId, number>;
}

/**
 * M4 из PLAN.md §1 — только Tariffs. В отличие от прототипа (Zan.dc.html:1074,
 * `payCustom` сразу завершал покупку), кнопка здесь только считает сумму и
 * открывает общую PaymentModal (M3) — тот же путь оплаты, что у пакетов и у Chat,
 * а не отдельный демо-шорткат в обход единственной точки оплаты.
 */
export function CustomOrderModal({
  open,
  onClose,
  dictionary,
  serviceIds,
  quantities,
  onQuantityChange,
  totalTenge,
  canConfirm,
  onConfirm,
  maxQty,
  unitPrices,
}: CustomOrderModalProps) {
  return (
    <Modal open={open} onClose={onClose} ariaLabel={dictionary.customModalTitle}>
      <h2 className="mb-1.5 text-h3 text-ink">{dictionary.customModalTitle}</h2>
      <p className="mb-4 text-body-sm text-muted">{dictionary.customModalSubtitle}</p>

      <div className="mb-4 flex flex-col gap-3">
        {serviceIds.map((serviceId) => {
          const name = dictionary.serviceName[serviceId];
          return (
            <div
              key={serviceId}
              className="flex items-center gap-2.5 rounded-lg border border-line p-3"
            >
              <div className="min-w-0 flex-1">
                <div className="text-body-sm font-semibold text-ink">{name}</div>
                <div className="text-caption text-muted">
                  {dictionary.unitPriceLabel(serviceId, unitPrices[serviceId])}
                </div>
              </div>
              <QuantityStepper
                value={quantities[serviceId]}
                max={maxQty}
                onChange={(qty) => onQuantityChange(serviceId, qty)}
                decreaseLabel={dictionary.decreaseQuantityLabel(name)}
                increaseLabel={dictionary.increaseQuantityLabel(name)}
                inputLabel={name}
              />
            </div>
          );
        })}
      </div>

      <div className="mb-4 flex items-center justify-between rounded-lg border border-line bg-bg p-3.5">
        <span className="text-body-sm text-ink">{dictionary.customTotalLabel}</span>
        <strong className="text-h3 text-ink">{formatTenge(totalTenge)}</strong>
      </div>

      <div className="flex flex-col gap-2">
        <Button onClick={onConfirm} disabled={!canConfirm}>
          {dictionary.customContinueCta}
        </Button>
        <Button variant="ghost" onClick={onClose}>
          {dictionary.cancel}
        </Button>
      </div>
    </Modal>
  );
}
