import { NavLink, useNavigate } from "react-router-dom";
import type { MouseEvent } from "react";
import { Moon, Sun } from "lucide-react";
import { cn } from "@/shared/lib/cn";
import { IconButton } from "@/shared/ui/IconButton";
import { useThemeStore } from "@/shared/stores/useThemeStore";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useUnsavedChangesStore } from "@/shared/stores/useUnsavedChangesStore";
import { navDictionary } from "@/shared/locales/nav";
import { BalanceIndicator } from "./BalanceIndicator";

const NAV_ITEMS = [
  { to: "/", key: "chat" as const },
  { to: "/history", key: "history" as const },
  { to: "/tariffs", key: "tariffs" as const },
  { to: "/settings", key: "settings" as const },
];

/**
 * Полная навигация видна от md и выше. На мобильном (< md) переходы между
 * разделами — через MobileNav (нижний таб-бар), здесь остаётся только
 * лого/баланс/язык/тема — см. PLAN.md Stage 0, п.3.
 */
export function Header() {
  const navigate = useNavigate();
  const theme = useThemeStore((state) => state.theme);
  const toggleTheme = useThemeStore((state) => state.toggleTheme);
  const lang = useLangStore((state) => state.lang);
  const toggleLang = useLangStore((state) => state.toggleLang);
  const t = navDictionary[lang];

  // Settings→Prompts (Stage 4a) может держать несохранённую форму — переход
  // из шапки на несохранённом экране спрашивает подтверждение, а не молча
  // теряет ввод (см. shared/stores/useUnsavedChangesStore.ts).
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
    <header className="sticky top-0 z-sticky border-b border-line bg-bg">
      <div className="mx-auto flex max-w-[1080px] items-center gap-3 px-4 py-3">
        <div className="mr-auto flex min-w-0 items-center gap-2.5">
          <span className="text-h3 font-bold text-ink">ZAN</span>
          <span className="hidden truncate font-mono text-micro text-muted sm:inline">
            {t.tagline}
          </span>
        </div>

        <nav className="hidden items-center gap-1.5 md:flex" aria-label={t.sectionsLabel}>
          {NAV_ITEMS.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === "/"}
              onClick={(event) => handleNavClick(event, item.to)}
              className={({ isActive }) =>
                cn(
                  "flex h-11 items-center rounded-md px-3.5 text-body-sm font-semibold transition-colors",
                  isActive ? "bg-accent-soft text-accent" : "text-muted hover:text-ink",
                )
              }
            >
              {t[item.key]}
            </NavLink>
          ))}
        </nav>

        <BalanceIndicator lang={lang} />

        <div className="flex items-center gap-1.5">
          <button
            type="button"
            onClick={toggleLang}
            title="Рус / Қаз"
            className="h-11 rounded-md border border-line px-3.5 text-body-sm text-muted hover:border-accent hover:text-accent"
          >
            {t.langLabel}
          </button>
          <IconButton aria-label={t.themeToggle} onClick={toggleTheme}>
            {theme === "light" ? (
              <Moon size={18} aria-hidden="true" />
            ) : (
              <Sun size={18} aria-hidden="true" />
            )}
          </IconButton>
        </div>
      </div>
    </header>
  );
}
