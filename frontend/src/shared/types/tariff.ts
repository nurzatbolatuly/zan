/**
 * Изначально был фичевым типом `features/tariffs/types.ts` (Stage 3) — обещание
 * "переезжает в shared/types, когда Settings→Tariffs (Stage 4b) тоже введёт
 * услуги" уже было записано в instructions.md «Общий словарь FE↔BE» до того,
 * как Stage 4b начался. Сейчас оба места (публичные тарифы и админский CRUD
 * тарифов — по сути один и тот же набор сущностей с двух сторон) используют
 * общий тип (FRONT_CODING_STANDARDS.md §5).
 *
 * Закрытый union, 1:1 с `openapi.yaml#ServiceId` (Stage 6) — до реального
 * бэка это был открытый `string` (Settings→Tariffs позволял создавать
 * произвольные услуги с `crypto.randomUUID()`-id), но реальный каталог
 * заведён миграцией и ограничен ровно этими двумя значениями: бэк не даёт
 * ни создавать, ни удалять услугу (`PUT /admin/services/{id}` — только
 * `price`/`is_active` для уже существующей), значит и на фронте это больше
 * не может быть открытым типом. `features/tariffs/types.ts#BuiltInServiceId`
 * стал избыточен как отдельный более узкий алиас — они теперь совпадают.
 */
export type ServiceId = "qa" | "doc";

export type PriceTable = Record<ServiceId, number>;

export interface BundleItem {
  serviceId: ServiceId;
  qty: number;
}

/**
 * Состав тарифа — язык-независимая структура, без `name`: у Stage 3
 * (публичные тарифы) имя — фиксированный demo-набор, лукапится по `id` из
 * `locales.ts#bundleName`; у Settings→Tariffs (Stage 4b, админ) тарифы
 * создаёт человек произвольно, имя хранится на самой сущности — см. `Bundle`
 * в `features/settings/types.ts` (`BundleTariff & { name }`), а не здесь.
 */
export interface BundleTariff {
  id: string;
  discountPercent: number;
  items: BundleItem[];
}
