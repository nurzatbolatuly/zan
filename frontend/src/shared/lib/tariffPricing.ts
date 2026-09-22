import type { BundleItem, BundleTariff, PriceTable } from "@/shared/types/tariff";

export interface BundlePrice {
  subtotalTenge: number;
  totalTenge: number;
  hasDiscount: boolean;
}

/**
 * Изначально жила только в `features/tariffs/pricing.ts` (Stage 3) — перенесена
 * сюда, когда Settings→Tariffs (Stage 4b) стал вторым потребителем той же
 * формулы скидки (округление до кратных 10 тенге, 1:1 с прототипом,
 * Zan.dc.html:1042 — "красивое" число мок-цены, не банковское округление денег).
 * `features/tariffs/pricing.ts` реэкспортирует эти функции — вызывающий код
 * Stage 3 не менялся.
 */
function roundToTens(value: number): number {
  return Math.round(value / 10) * 10;
}

export function computeItemsSubtotal(items: BundleItem[], prices: PriceTable): number {
  // `?? 0` — не бизнес-правило, а следствие открытого `ServiceId` (shared/types/tariff.ts):
  // для реально существующей услуги цена в таблице есть всегда, но
  // `noUncheckedIndexedAccess` больше не может этого гарантировать по типу.
  return items.reduce((sum, item) => sum + (prices[item.serviceId] ?? 0) * item.qty, 0);
}

export function computeBundlePrice(
  bundle: Pick<BundleTariff, "items" | "discountPercent">,
  prices: PriceTable,
): BundlePrice {
  const subtotalTenge = computeItemsSubtotal(bundle.items, prices);
  const hasDiscount = bundle.discountPercent > 0;
  const totalTenge = hasDiscount
    ? roundToTens(subtotalTenge * (1 - bundle.discountPercent / 100))
    : subtotalTenge;
  return { subtotalTenge, totalTenge, hasDiscount };
}
