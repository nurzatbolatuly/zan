// ServiceId/PriceTable/BundleItem/BundleTariff переехали в shared/types/tariff.ts,
// когда Settings→Tariffs (Stage 4b) тоже завёл услуги — см. instructions.md
// «Общий словарь FE↔BE». Реэкспорт, чтобы не трогать вызывающий код этой фичи
// (pricing.ts, useTariffs.ts, components/*, locales.ts всё ещё импортируют из "./types").
export type {
  ServiceId,
  PriceTable,
  BundleItem,
  BundleTariff,
} from "@/shared/types/tariff";

import type { ServiceId } from "@/shared/types/tariff";

/**
 * До Stage 6 здесь был отдельный `BuiltInServiceId = "qa" | "doc"` — более
 * узкий локальный алиас, нужный, пока `shared/types/tariff.ts#ServiceId` был
 * открытым `string` (админ мог завести произвольную услугу). Реальный бэк
 * закрыл каталог услуг ровно этими двумя значениями (`ServiceId` сам теперь
 * `"qa" | "doc"`, см. shared/types/tariff.ts) — отдельный алиас с тем же
 * значением стал бы дублирующей абстракцией, убран, `ServiceId` используется
 * напрямую везде в этой фиче.
 */
export type CustomOrderQuantities = Record<ServiceId, number>;
