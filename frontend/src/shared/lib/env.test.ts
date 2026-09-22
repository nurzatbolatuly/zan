import { afterEach, describe, expect, it, vi } from "vitest";
import { getApiBaseUrl, getApiBaseUrlSafe } from "./env";

afterEach(() => {
  vi.unstubAllEnvs();
});

describe("getApiBaseUrl", () => {
  it("убирает хвостовой слэш, чтобы не было двойного слэша при склейке с путём", () => {
    vi.stubEnv("VITE_API_BASE_URL", "https://api.zan.kz/");
    expect(getApiBaseUrl()).toBe("https://api.zan.kz");
  });

  it("бросает понятную ошибку, если переменная не задана (пустой прод-деплой)", () => {
    vi.stubEnv("VITE_API_BASE_URL", "");
    expect(() => getApiBaseUrl()).toThrow("VITE_API_BASE_URL");
  });
});

describe("getApiBaseUrlSafe", () => {
  it("возвращает null вместо исключения — логирование не должно падать", () => {
    vi.stubEnv("VITE_API_BASE_URL", "");
    expect(getApiBaseUrlSafe()).toBeNull();
  });
});
