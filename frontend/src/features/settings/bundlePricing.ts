import { computeBundlePrice as computeBundlePriceShared } from "@/shared/lib/tariffPricing";
import type { BundlePrice } from "@/shared/lib/tariffPricing";
import type { PriceTable, ServiceId } from "@/shared/types/tariff";
import type { Bundle, BundleItem, Service } from "./types";

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

/**
 * Первый тариф, в состав которого реально входит данная услуга (qty > 0) —
 * используется, чтобы заблокировать удаление услуги из `useAdminTariffs.ts`:
 * удалить услугу, которая используется хотя бы в одном тарифе, нельзя — иначе
 * итоговая цена этого тарифа молча "усохнет" без объяснения.
 */
export function findBundleUsingService(
  bundles: Bundle[],
  serviceId: ServiceId,
): Bundle | undefined {
  return bundles.find((bundle) =>
    bundle.items.some((item) => item.serviceId === serviceId && item.qty > 0),
  );
}
