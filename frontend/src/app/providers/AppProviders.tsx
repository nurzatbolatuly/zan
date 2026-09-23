import { useEffect, useRef } from "react";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useThemeStore } from "@/shared/stores/useThemeStore";
import { useLangStore } from "@/shared/stores/useLangStore";
import { api, ensureSession } from "@/shared/lib/api";
import { logger } from "@/shared/lib/logger";
import type { Session } from "@/shared/types/api";

// staleTime не задан намеренно (дефолт TanStack — 0): серверные данные
// (баланс, треды, история) считаются устаревшими сразу и перечитываются
// при каждом монтировании/возврате фокуса/переподключении сети — экран не
// показывает значение, которое сервер уже изменил. Уже загруженное
// показывается только на время этого перечитывания, не вместо него.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
    },
  },
});

/** Применяет data-theme на <html> при каждом изменении темы в сторе. */
function ThemeSync() {
  const theme = useThemeStore((state) => state.theme);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);
  return null;
}

/**
 * Bootstrap анонимной сессии (Stage 6) — один раз на старте приложения:
 * `ensureSession()` создаёт токен, если его ещё нет (см. shared/lib/api.ts),
 * затем `GET /sessions/me` синхронизирует локальные тема/язык с серверными
 * (сервер — источник истины для уже существующей сессии, brief/PLAN.md §9).
 * После этого локальные переключения темы/языка идут в обратную сторону —
 * `PATCH /sessions/me` (fire-and-forget, ошибка только логируется: локальный
 * UI не должен ждать сеть ради переключения темы). `isHydratingRef` не даёт
 * начальной синхронизации самой себя тут же отправить обратно на бэк тем же
 * значением.
 */
function SessionBoot() {
  const isHydratingRef = useRef(false);

  useEffect(() => {
    let cancelled = false;
    void ensureSession()
      .then(() => api.get<Session>("/sessions/me"))
      .then((session) => {
        if (cancelled) return;
        isHydratingRef.current = true;
        useLangStore.getState().setLang(session.language);
        if (session.theme) useThemeStore.getState().setTheme(session.theme);
        isHydratingRef.current = false;
      })
      .catch((error: unknown) => {
        logger.error({ scope: "session", event: "bootstrap_failed", error });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const unsubscribeLang = useLangStore.subscribe((state, prevState) => {
      if (state.lang === prevState.lang || isHydratingRef.current) return;
      api
        .patch("/sessions/me", { language: state.lang })
        .catch((error: unknown) =>
          logger.error({ scope: "session", event: "patch_language_failed", error }),
        );
    });
    const unsubscribeTheme = useThemeStore.subscribe((state, prevState) => {
      if (state.theme === prevState.theme || isHydratingRef.current) return;
      api
        .patch("/sessions/me", { theme: state.theme })
        .catch((error: unknown) =>
          logger.error({ scope: "session", event: "patch_theme_failed", error }),
        );
    });
    return () => {
      unsubscribeLang();
      unsubscribeTheme();
    };
  }, []);

  return null;
}

export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeSync />
      <SessionBoot />
      {children}
    </QueryClientProvider>
  );
}
