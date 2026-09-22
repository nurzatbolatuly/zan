import type { ThreadStatus } from "@/shared/types/common";
import type { BundleItem, BundleTariff, ServiceId } from "@/shared/types/tariff";

export type { ServiceId, BundleItem };

export type SettingsTabKey = "prompts" | "tariffs" | "analytics";

/**
 * Ровно два системных агента — фиксировано продуктом (backend-roadmap.md §1.1:
 * "Q&A-агент" и "агент Документы"), не список с CRUD, поэтому ключ — union,
 * не `string` (PLAN.md §5 Stage 4a: "2 textarea", не N textarea).
 */
export type AgentPromptKey = "qa" | "documents";

export interface AgentPrompt {
  key: AgentPromptKey;
  label: string;
  hint: string;
  value: string;
}

export interface Service {
  id: ServiceId;
  typeLabel: string;
  name: string;
  unitPriceTenge: number;
}

/** Результат отправки формы `ServiceCreateModal` — без `id`, его назначает `useAdminTariffs` (тот же приём, что `BundleFormResult`). */
export interface ServiceFormResult {
  typeLabel: string;
  name: string;
  unitPriceTenge: number;
}

/**
 * У Stage 3 (публичные тарифы) `BundleTariff` не хранит `name` — имя там
 * фиксированный demo-набор, лукапится по `id` из `locales.ts#bundleName`
 * (shared/types/tariff.ts). Здесь тариф создаёт админ произвольно — лукапить
 * имя неоткуда, оно хранится на самой сущности.
 */
export interface Bundle extends BundleTariff {
  name: string;
}

/** Результат отправки формы `BundleEditModal` (M2) — без `id`, его назначает `useAdminTariffs`. */
export interface BundleFormResult {
  name: string;
  discountPercent: number;
  items: BundleItem[];
}

export interface AnalyticsStatusRow {
  status: ThreadStatus;
  count: number;
}

export interface AnalyticsSummary {
  totalRequests: number;
  /** Уже отформатированная строка ("38 сек") — единица времени на бэке не зафиксирована, см. «Открытые вопросы». */
  avgResponseTimeLabel: string;
  satisfactionRateLabel: string;
  byStatus: AnalyticsStatusRow[];
}
