import { describe, expect, it } from "vitest";
import { computeCustomOrderPrice, quantityOf } from "./pricing";
import type { BuiltInServiceId } from "./types";

// computeBundlePrice — теперь shared/lib/tariffPricing.test.ts (см. pricing.ts:
// эта фича реэкспортирует функцию, а не определяет её заново).

const PRICES: Record<BuiltInServiceId, number> = { qa: 2900, doc: 4900 };

describe("computeCustomOrderPrice", () => {
  it("sums quantities across services without any discount", () => {
    expect(computeCustomOrderPrice({ qa: 2, doc: 1 }, PRICES)).toBe(2 * 2900 + 4900);
  });

  it("returns 0 when every quantity is 0", () => {
    expect(computeCustomOrderPrice({ qa: 0, doc: 0 }, PRICES)).toBe(0);
  });
});

describe("quantityOf", () => {
  it("sums quantities of a given service across items", () => {
    expect(
      quantityOf(
        [
          { serviceId: "qa", qty: 2 },
          { serviceId: "doc", qty: 1 },
        ],
        "qa",
      ),
    ).toBe(2);
  });

  it("returns 0 when the service is not part of the bundle", () => {
    expect(quantityOf([{ serviceId: "doc", qty: 3 }], "qa")).toBe(0);
  });
});
