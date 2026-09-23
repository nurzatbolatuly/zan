import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useElapsedMs } from "./useElapsedMs";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useElapsedMs", () => {
  it("не считает, пока не запущен", () => {
    const { result } = renderHook(() => useElapsedMs(false));

    act(() => {
      vi.advanceTimersByTime(5_000);
    });
    expect(result.current).toBe(0);
  });

  it("растёт с каждым тиком после запуска", () => {
    const { result } = renderHook(() => useElapsedMs(true));

    act(() => {
      vi.advanceTimersByTime(3_000);
    });
    expect(result.current).toBe(3_000);
  });

  it("остановка сбрасывает в 0, новый запуск считает с нуля", () => {
    const { result, rerender } = renderHook(({ isRunning }) => useElapsedMs(isRunning), {
      initialProps: { isRunning: true },
    });
    act(() => {
      vi.advanceTimersByTime(4_000);
    });

    rerender({ isRunning: false });
    expect(result.current).toBe(0);

    rerender({ isRunning: true });
    act(() => {
      vi.advanceTimersByTime(1_000);
    });
    expect(result.current).toBe(1_000);
  });
});
