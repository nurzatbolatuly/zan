import type { BundleTariff, BuiltInServiceId } from "./types";

/**
 * Цены услуг — тот же демо-набор, что и в features/chat/mocks.ts
 * (CONSULTATION_PRICE_TENGE/DOCUMENT_PRICE_TENGE, источник — Zan.dc.html:754-762).
 * Не переиспользуется оттуда напрямую: Stage 1-4 намеренно независимы друг от
 * друга и оба работают на своих моках (PLAN.md §5) — единственный настоящий
 * источник цены появится в Stage 6 (`/tariffs`), тогда оба места заменятся им.
 * `Record<BuiltInServiceId, ...>`, не открытый `PriceTable` — эта страница
 * знает ровно про две зашитые услуги (см. types.ts#BuiltInServiceId).
 */
export const SERVICE_PRICES: Record<BuiltInServiceId, number> = { qa: 2900, doc: 4900 };

/** Максимальное количество одной услуги в своём наборе — 1:1 с прототипом (CATEGORY_MAX, Zan.dc.html:752). */
export const CUSTOM_ORDER_MAX_QTY = 20;

/**
 * Состав пакетов — 1:1 из прототипа (BUNDLES, Zan.dc.html:765-777). Локализованные
 * названия — в locales.ts (`bundleName`, ключ по `id`), это UI-текст, а не структура данных.
 */
export const BUNDLES: BundleTariff[] = [
  { id: "b1", discountPercent: 0, items: [{ serviceId: "qa", qty: 1 }] },
  { id: "b2", discountPercent: 15, items: [{ serviceId: "qa", qty: 3 }] },
  {
    id: "b3",
    discountPercent: 10,
    items: [
      { serviceId: "qa", qty: 2 },
      { serviceId: "doc", qty: 1 },
    ],
  },
  {
    id: "b4",
    discountPercent: 25,
    items: [
      { serviceId: "qa", qty: 10 },
      { serviceId: "doc", qty: 3 },
    ],
  },
];
