import { describe, expect, it } from "vitest";
import { calcBundlePrice, findBundleUsingService } from "./bundlePricing";
import type { Bundle, Service } from "./types";

// Формула скидки/округления — уже покрыта shared/lib/tariffPricing.test.ts.
// Здесь — только адаптация Service[] (админская форма цены) к PriceTable.

const SERVICES: Service[] = [
  { id: "qa", typeLabel: "Консультация", name: "Вопрос-ответ", unitPriceTenge: 2900 },
  {
    id: "doc",
    typeLabel: "Документ",
    name: "Подготовка документа",
    unitPriceTenge: 4900,
  },
];

describe("calcBundlePrice", () => {
  it("считает сумму по цене из Service[], без скидки", () => {
    const price = calcBundlePrice([{ serviceId: "qa", qty: 3 }], SERVICES, 0);
    expect(price).toEqual({ subtotalTenge: 8700, totalTenge: 8700, hasDiscount: false });
  });

  it("учитывает отредактированную (не из мока) цену услуги", () => {
    const editedServices: Service[] = [
      { ...SERVICES[0]!, unitPriceTenge: 3000 },
      SERVICES[1]!,
    ];
    const price = calcBundlePrice([{ serviceId: "qa", qty: 2 }], editedServices, 0);
    expect(price.subtotalTenge).toBe(6000);
  });

  it("считает сумму по нескольким услугам со скидкой", () => {
    const price = calcBundlePrice(
      [
        { serviceId: "qa", qty: 2 },
        { serviceId: "doc", qty: 1 },
      ],
      SERVICES,
      10,
    );
    expect(price.subtotalTenge).toBe(2900 * 2 + 4900);
    expect(price.hasDiscount).toBe(true);
  });
});

const BUNDLES: Bundle[] = [
  { id: "b1", name: "1 вопрос", discountPercent: 0, items: [{ serviceId: "qa", qty: 1 }] },
  {
    id: "b3",
    name: "Вопрос + документ",
    discountPercent: 10,
    items: [
      { serviceId: "qa", qty: 2 },
      { serviceId: "doc", qty: 1 },
    ],
  },
];

describe("findBundleUsingService", () => {
  it("находит первый тариф, где услуга реально входит в состав (qty > 0)", () => {
    expect(findBundleUsingService(BUNDLES, "doc")).toBe(BUNDLES[1]);
  });

  it("возвращает undefined, если услуга не используется ни в одном тарифе", () => {
    expect(findBundleUsingService(BUNDLES, "new-service-id")).toBeUndefined();
  });

  it("игнорирует позицию с qty=0 (услуга исключена из тарифа)", () => {
    const bundlesWithZeroQty: Bundle[] = [
      { id: "b5", name: "Тест", discountPercent: 0, items: [{ serviceId: "doc", qty: 0 }] },
    ];
    expect(findBundleUsingService(bundlesWithZeroQty, "doc")).toBeUndefined();
  });
});
