import { describe, expect, it } from "vitest";
import { computeBundlePrice } from "./tariffPricing";
import type { PriceTable } from "@/shared/types/tariff";

const PRICES: PriceTable = { qa: 2900, doc: 4900 };

describe("computeBundlePrice", () => {
  it("returns the plain subtotal when there is no discount", () => {
    const price = computeBundlePrice(
      { discountPercent: 0, items: [{ serviceId: "qa", qty: 1 }] },
      PRICES,
    );
    expect(price).toEqual({ subtotalTenge: 2900, totalTenge: 2900, hasDiscount: false });
  });

  it("applies the discount and rounds to the nearest 10 tenge", () => {
    // 3 * 2900 = 8700, -15% = 7395 -> rounds to 7400 (matches Zan.dc.html bundleVals rounding)
    const price = computeBundlePrice(
      { discountPercent: 15, items: [{ serviceId: "qa", qty: 3 }] },
      PRICES,
    );
    expect(price).toEqual({ subtotalTenge: 8700, totalTenge: 7400, hasDiscount: true });
  });

  it("sums multiple services in one bundle", () => {
    const price = computeBundlePrice(
      {
        discountPercent: 10,
        items: [
          { serviceId: "qa", qty: 2 },
          { serviceId: "doc", qty: 1 },
        ],
      },
      PRICES,
    );
    // subtotal = 2*2900 + 4900 = 10700, -10% = 9630 -> rounds to 9630
    expect(price).toEqual({ subtotalTenge: 10700, totalTenge: 9630, hasDiscount: true });
  });

  it("returns 0 for an empty bundle", () => {
    expect(computeBundlePrice({ discountPercent: 20, items: [] }, PRICES)).toEqual({
      subtotalTenge: 0,
      totalTenge: 0,
      hasDiscount: true,
    });
  });

  it("treats a service missing from the price table as 0, not NaN", () => {
    const price = computeBundlePrice(
      { discountPercent: 0, items: [{ serviceId: "unknown", qty: 5 }] },
      PRICES,
    );
    expect(price).toEqual({ subtotalTenge: 0, totalTenge: 0, hasDiscount: false });
  });
});
