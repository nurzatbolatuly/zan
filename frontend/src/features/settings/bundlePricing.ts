import { computeBundlePrice as computeBundlePriceShared } from "@/shared/lib/tariffPricing";
import type { BundlePrice } from "@/shared/lib/tariffPricing";
import type { PriceTable } from "@/shared/types/tariff";
import type { BundleItem, Service } from "./types";

function toPriceTable(services: Service[]): PriceTable {
  const table = {} as PriceTable;
  services.forEach((service) => {
    table[service.id] = service.unitPriceTenge;
  });
  return table;
}

/**
 * Обёртка над общей `shared/lib/tariffPricing.ts#computeBundlePrice` (та же
 * формула, что и у Stage 3/публичных тарифов) — здесь только адаптация
 * админского `Service[]` (id + текущая цена, редактируемая инлайн) к плоской
 * `PriceTable`, которую понимает общая функция.
 */
export function calcBundlePrice(
  items: BundleItem[],
  services: Service[],
  discountPercent: number,
): BundlePrice {
  return computeBundlePriceShared({ items, discountPercent }, toPriceTable(services));
}
