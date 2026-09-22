import type { ReactNode } from "react";
import { Header } from "./Header";
import { MobileNav } from "./MobileNav";
import { ConfirmModal } from "@/shared/ui/ConfirmModal";
import { PaymentModal } from "@/shared/ui/PaymentModal";
import { ToastViewport } from "@/shared/ui/toast/ToastViewport";

/**
 * Общая оболочка для всех страниц (PLAN.md Stage 0, п.3). Общие модалки
 * (M1 ConfirmModal, M3 PaymentModal) и ToastViewport смонтированы здесь один
 * раз — страницы их не рендерят повторно, только открывают через сторы.
 */
export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen bg-bg text-ink">
      <Header />
      <main className="mx-auto max-w-[1080px] px-4 pb-[var(--mobile-nav-h)] pt-2 md:pb-8">
        {children}
      </main>
      <MobileNav />
      <ConfirmModal />
      <PaymentModal />
      <ToastViewport />
    </div>
  );
}
