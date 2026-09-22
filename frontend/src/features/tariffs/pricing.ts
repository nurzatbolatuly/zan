import type { BundleItem, BuiltInServiceId, CustomOrderQuantities, ServiceId } from "./types";

// computeBundlePrice/computeItemsSubtotal/BundlePrice переехали в
// shared/lib/tariffPricing.ts, когда Settings→Tariffs (Stage 4b) стал вторым
// потребителем той же формулы скидки (instructions.md «Конвенции») —
// реэкспорт, чтобы не трогать вызывающий код этой фичи (useTariffs.ts,
// components/BundleCard.tsx, pricing.test.ts).
export { computeBundlePrice, computeItemsSubtotal } from "@/shared/lib/tariffPricing";
export type { BundlePrice } from "@/shared/lib/tariffPricing";

export function computeCustomOrderPrice(
  quantities: CustomOrderQuantities,
  prices: Record<BuiltInServiceId, number>,
): number {
  return (Object.keys(quantities) as BuiltInServiceId[]).reduce(
    (sum, serviceId) => sum + prices[serviceId] * quantities[serviceId],
    0,
  );
}

/** Сколько единиц данной услуги входит в пакет — нужно, чтобы понять, на сколько
 * консультаций пополнить баланс после покупки (баланс в шапке считает только `qa`). */
export function quantityOf(items: BundleItem[], serviceId: ServiceId): number {
  return items
    .filter((item) => item.serviceId === serviceId)
    .reduce((sum, item) => sum + item.qty, 0);
}
