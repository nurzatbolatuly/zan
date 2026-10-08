import { useState } from "react";
import { EmptyState, Tabs } from "@/shared/ui";
import { useRole } from "@/shared/hooks/useRole";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useUnsavedChangesStore } from "@/shared/stores/useUnsavedChangesStore";
import { navDictionary } from "@/shared/locales/nav";
import { settingsDictionary } from "./locales";
import { AdminGate } from "./components/AdminGate";
import { PromptsTab } from "./components/PromptsTab";
import { TariffsTab } from "./components/TariffsTab";
import { TemplatesTab } from "./components/TemplatesTab";
import { AnalyticsTab } from "./components/AnalyticsTab";
import type { SettingsTabKey } from "./types";

/**
 * Stage 4 (PLAN.md §5) — заменяет плейсхолдер Stage 0 целиком
 * (FRONT_CODING_STANDARDS.md §3). 4 независимые вкладки — общая только
 * `useRole()`-гейт (Stage 0) и переключатель вкладок, вся логика каждой
 * вкладки живёт в своём компоненте/хуке.
 */
export function SettingsPage() {
  const role = useRole();
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang].common;
  const [tab, setTab] = useState<SettingsTabKey>("prompts");

  if (role !== "admin") {
    return (
      <div className="pt-6">
        <EmptyState title={t.accessDeniedTitle} description={t.accessDeniedBody} />
      </div>
    );
  }

  // Уход со вкладки «Промпты» с несохранённой формой — подтверждение, а не
  // молчаливая потеря ввода (см. shared/stores/useUnsavedChangesStore.ts).
  // Copy — общая с шапкой/таб-баром (shared/locales/nav.ts), это один и тот
  // же UX-паттерн "есть несохранённое, точно уйти?", не отдельная формулировка.
  const nav = navDictionary[lang];
  function handleTabChange(key: string) {
    useUnsavedChangesStore.getState().guard(() => setTab(key as SettingsTabKey), {
      title: nav.unsavedTitle,
      confirmLabel: nav.unsavedConfirm,
      cancelLabel: nav.unsavedCancel,
    });
  }

  return (
    <div className="max-w-[860px] pt-6">
      <h1 className="mb-1.5 text-h1 text-ink">{t.title}</h1>
      <p className="mb-5 text-body text-muted">{t.subtitle}</p>

      <AdminGate>
        <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
          <Tabs
            layout="sidebar"
            activeKey={tab}
            onChange={handleTabChange}
            items={[
              { key: "prompts", label: t.tabPrompts },
              { key: "tariffs", label: t.tabTariffs },
              { key: "templates", label: t.tabTemplates },
              { key: "analytics", label: t.tabAnalytics },
            ]}
          />

          <div className="min-w-0 flex-1">
            {tab === "prompts" && <PromptsTab />}
            {tab === "tariffs" && <TariffsTab />}
            {tab === "templates" && <TemplatesTab />}
            {tab === "analytics" && <AnalyticsTab />}
          </div>
        </div>
      </AdminGate>
    </div>
  );
}
