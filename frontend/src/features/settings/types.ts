import type { ThreadStatus } from "@/shared/types/common";
import type { BundleItem, ServiceId } from "@/shared/types/tariff";

export type { ServiceId, BundleItem };

export type SettingsTabKey = "prompts" | "tariffs" | "templates" | "analytics";

/**
 * Ровно два системных агента (openapi.yaml#AgentType) — фиксировано
 * продуктом, не список с CRUD, поэтому ключ — union, не `string`. Значение
 * — единственное число ("document"), 1:1 с бэком (Stage 6 переименовал из
 * "documents" — см. instructions.md, расхождение №5 плана интеграции).
 */
export type AgentPromptKey = "qa" | "document";

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
  isActive: boolean;
}

/**
 * Тариф — реальная сущность с бэка (`GET/POST/PUT/DELETE /admin/tariffs`,
 * Stage 6), имя вводит админ и хранится на самой сущности (в отличие от
 * Stage 3 `BundleTariff`, где имя — demo-lookup по фиксированному id).
 */
export interface Bundle {
  id: string;
  name: string;
  discountPercent: number;
  items: BundleItem[];
}

/** Результат отправки формы `BundleEditModal` (M2) — без `id`, назначает бэк. */
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
  /** `null` — данных ещё нет (openapi.yaml: `omitempty`, не `0`), не путать с реальным "0 сек"/"0%". */
  avgResponseTimeLabel: string | null;
  satisfactionRateLabel: string | null;
  byStatus: AnalyticsStatusRow[];
}

/** Тип документа из справочника (`/admin/document-types`). */
export interface DocumentType {
  id: string;
  name: string;
}

export type TemplateFileKind = "pdf" | "docx";

/** Шаблон документа (`/admin/document-templates`). */
export interface DocumentTemplate {
  id: string;
  typeId: string;
  title: string;
  originalName: string;
  fileKind: TemplateFileKind;
  sizeBytes: number;
  /** PDF-версия для просмотра (у DOCX — копия, построенная бэком). */
  previewUrl: string;
  updatedAt: string;
}

/** Результат формы `TemplateEditModal`; `file: null` при изменении — файл не меняется. */
export interface TemplateFormResult {
  typeId: string;
  title: string;
  file: File | null;
}

export interface TemplateGroup {
  type: DocumentType;
  templates: DocumentTemplate[];
}
