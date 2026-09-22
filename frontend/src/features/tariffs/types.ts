// ServiceId/PriceTable/BundleItem/BundleTariff переехали в shared/types/tariff.ts,
// когда Settings→Tariffs (Stage 4b) тоже завёл услуги — см. instructions.md
// «Общий словарь FE↔BE» (снят статус "переедет, когда..."). Реэкспорт, чтобы
// не трогать вызывающий код этой фичи (mocks.ts, pricing.ts, useTariffs.ts,
// components/*, locales.ts всё ещё импортируют из "./types").
export type {
  ServiceId,
  PriceTable,
  BundleItem,
  BundleTariff,
} from "@/shared/types/tariff";

/**
 * `shared/types/tariff.ts#ServiceId` открыт (`string`) ради Settings→Tariffs
 * (админ может создать услугу с произвольным id). Эта страница (Stage 3) —
 * публичный демо-каталог, работает только с двумя зашитыми услугами и их
 * склонением ("1 запрос" / "2 запроса" — см. locales.ts), поэтому берёт свой,
 * более узкий алиас вместо открытого `ServiceId` там, где важна гарантия
 * "ключ есть всегда" (иначе `noUncheckedIndexedAccess` потребовал бы `?? ""`
 * по всей странице без единой реальной причины — набор всегда fixed).
 */
export type BuiltInServiceId = "qa" | "doc";

export type CustomOrderQuantities = Record<BuiltInServiceId, number>;
