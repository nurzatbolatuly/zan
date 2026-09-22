import { NavLink, useNavigate } from "react-router-dom";
import type { MouseEvent } from "react";
import { MessageCircle, History, Wallet, Settings } from "lucide-react";
import { cn } from "@/shared/lib/cn";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useUnsavedChangesStore } from "@/shared/stores/useUnsavedChangesStore";
import { navDictionary } from "@/shared/locales/nav";

const ITEMS = [
  { to: "/", key: "chat" as const, icon: MessageCircle },
  { to: "/history", key: "history" as const, icon: History },
  { to: "/tariffs", key: "tariffs" as const, icon: Wallet },
  { to: "/settings", key: "settings" as const, icon: Settings },
];

/**
 * Нижний таб-бар, виден только < md (PLAN.md Stage 0, п.3). Высота
 * зарезервирована через CSS-переменную --mobile-nav-h (shared/styles/index.css) —
 * контент страниц не прячется под бар.
 *
 * Контракт для Stage 1 (Chat): композер — тоже fixed-элемент внизу экрана,
 * он должен позиционироваться с учётом той же --mobile-nav-h
 * (`bottom-[var(--mobile-nav-h)] md:bottom-0`), а не поверх этого бара —
 * бар остаётся видимым и на экране чата, это единственный способ на мобильном
 * уйти в Историю/Тарифы/Настройки, раз верхняя навигация скрыта до md.
 */
export function MobileNav() {
  const navigate = useNavigate();
  const lang = useLangStore((state) => state.lang);
  const t = navDictionary[lang];

  // Тот же гейт, что и в Header.tsx — см. shared/stores/useUnsavedChangesStore.ts.
  function handleNavClick(event: MouseEvent, to: string) {
    if (!useUnsavedChangesStore.getState().isDirty) return;
    event.preventDefault();
    useUnsavedChangesStore.getState().guard(() => navigate(to), {
      title: t.unsavedTitle,
      confirmLabel: t.unsavedConfirm,
      cancelLabel: t.unsavedCancel,
    });
  }

  return (
    <nav
      aria-label={t.sectionsLabel}
      className="safe-area-bottom fixed inset-x-0 bottom-0 z-sticky flex border-t border-line bg-surface md:hidden"
    >
      {ITEMS.map(({ to, key, icon: Icon }) => (
        <NavLink
          key={to}
          to={to}
          end={to === "/"}
          onClick={(event) => handleNavClick(event, to)}
          className={({ isActive }) =>
            cn(
              "flex flex-1 flex-col items-center gap-1 py-2 text-micro font-mono",
              isActive ? "text-accent" : "text-muted",
            )
          }
        >
          <Icon size={20} aria-hidden="true" />
          {t[key]}
        </NavLink>
      ))}
    </nav>
  );
}
