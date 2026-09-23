import type { CustomOrderQuantities, ServiceId } from "./types";

// computeBundlePrice/computeItemsSubtotal/BundlePrice переехали в
// shared/lib/tariffPricing.ts, когда Settings→Tariffs (Stage 4b) стал вторым
// потребителем той же формулы скидки (instructions.md «Конвенции») —
// реэкспорт, чтобы не трогать вызывающий код этой фичи (components/BundleCard.tsx).
// Публичная витрина (Stage 6) больше не пересчитывает bundle-цену сама —
// `GET /tariffs` уже отдаёт готовые subtotal/total (openapi.yaml#Tariff) —
// но `BundlePrice`/`computeBundlePrice` остаются нужны для live-превью
// суммы в CustomOrderModal, где ответа сервера ещё нет.
export { computeBundlePrice, computeItemsSubtotal } from "@/shared/lib/tariffPricing";
export type { BundlePrice } from "@/shared/lib/tariffPricing";

export function computeCustomOrderPrice(
  quantities: CustomOrderQuantities,
  prices: Record<ServiceId, number>,
): number {
  return (Object.keys(quantities) as ServiceId[]).reduce(
    (sum, serviceId) => sum + prices[serviceId] * quantities[serviceId],
    0,
  );
}
