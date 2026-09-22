import { lazy, Suspense } from "react";
import type { ReactElement } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "./layout/AppShell";
import { ErrorBoundary } from "./ErrorBoundary";
import { NotFoundPage } from "./NotFoundPage";
import { Skeleton } from "@/shared/ui/Skeleton";

// Ленивая загрузка на страницу (PLAN.md §3 — каждая страница свой чанк,
// реально независимая сборка от остальных стадий).
const ChatPage = lazy(() =>
  import("@/features/chat/ChatPage").then((m) => ({ default: m.ChatPage })),
);
const HistoryPage = lazy(() =>
  import("@/features/history/HistoryPage").then((m) => ({ default: m.HistoryPage })),
);
const TariffsPage = lazy(() =>
  import("@/features/tariffs/TariffsPage").then((m) => ({ default: m.TariffsPage })),
);
const SettingsPage = lazy(() =>
  import("@/features/settings/SettingsPage").then((m) => ({ default: m.SettingsPage })),
);

function RouteFallback() {
  return (
    <div className="flex flex-col gap-3 pt-6">
      <Skeleton className="h-8 w-2/3" />
      <Skeleton className="h-40 w-full" />
    </div>
  );
}

function withBoundary(scope: string, element: ReactElement) {
  return (
    <ErrorBoundary scope={scope}>
      <Suspense fallback={<RouteFallback />}>{element}</Suspense>
    </ErrorBoundary>
  );
}

export function AppRouter() {
  return (
    <AppShell>
      <Routes>
        <Route path="/" element={withBoundary("route.chat", <ChatPage />)} />
        <Route path="/history" element={withBoundary("route.history", <HistoryPage />)} />
        <Route path="/tariffs" element={withBoundary("route.tariffs", <TariffsPage />)} />
        <Route
          path="/settings"
          element={withBoundary("route.settings", <SettingsPage />)}
        />
        <Route path="/404" element={<NotFoundPage />} />
        <Route path="*" element={<Navigate to="/404" replace />} />
      </Routes>
    </AppShell>
  );
}
