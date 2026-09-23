import { describe, expect, it } from "vitest";
import { resolveReplyProgressStep } from "./replyProgress";

describe("resolveReplyProgressStep", () => {
  it("пока POST в полёте — всегда sending, независимо от времени", () => {
    expect(
      resolveReplyProgressStep({ isSending: true, hasFile: true, elapsedMs: 60_000 }),
    ).toBe("sending");
  });

  it("без файла идёт analyzing → checkingLaw → composing → takingLonger", () => {
    const at = (elapsedMs: number) =>
      resolveReplyProgressStep({ isSending: false, hasFile: false, elapsedMs });

    expect(at(0)).toBe("analyzing");
    expect(at(4_999)).toBe("analyzing");
    expect(at(5_000)).toBe("checkingLaw");
    expect(at(14_000)).toBe("composing");
    expect(at(30_000)).toBe("takingLonger");
    expect(at(600_000)).toBe("takingLonger");
  });

  it("с файлом сначала readingFile, остальные этапы сдвигаются на его длительность", () => {
    const at = (elapsedMs: number) =>
      resolveReplyProgressStep({ isSending: false, hasFile: true, elapsedMs });

    expect(at(0)).toBe("readingFile");
    expect(at(6_000)).toBe("analyzing");
    expect(at(11_000)).toBe("checkingLaw");
    expect(at(36_000)).toBe("takingLonger");
  });
});
