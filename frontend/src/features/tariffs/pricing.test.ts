import { describe, expect, it } from "vitest";
import { computeCustomOrderPrice } from "./pricing";
import type { ServiceId } from "./types";

// computeBundlePrice — теперь shared/lib/tariffPricing.test.ts (см. pricing.ts:
// эта фича реэкспортирует функцию, а не определяет её заново).

const PRICES: Record<ServiceId, number> = { qa: 2900, doc: 4900 };

describe("computeCustomOrderPrice", () => {
  it("sums quantities across services without any discount", () => {
    expect(computeCustomOrderPrice({ qa: 2, doc: 1 }, PRICES)).toBe(2 * 2900 + 4900);
  });

  it("returns 0 when every quantity is 0", () => {
    expect(computeCustomOrderPrice({ qa: 0, doc: 0 }, PRICES)).toBe(0);
  });
});
