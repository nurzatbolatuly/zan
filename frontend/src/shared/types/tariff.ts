/**
 * Изначально был фичевым типом `features/tariffs/types.ts` (Stage 3) — обещание
 * "переезжает в shared/types, когда Settings→Tariffs (Stage 4b) тоже введёт
 * услуги" уже было записано в instructions.md «Общий словарь FE↔BE» до того,
 * как Stage 4b начался. Сейчас оба места (публичные тарифы и админский CRUD
 * тарифов — по сути один и тот же набор сущностей с двух сторон) используют
 * общий тип (FRONT_CODING_STANDARDS.md §5).
 *
 * Открытый `string`, не закрытый union — Settings→Tariffs (Stage 4b) даёт
 * админу создавать произвольные услуги (`crypto.randomUUID()` как id, тот же
 * приём, что уже используют новые тарифы в `useAdminTariffs.ts#saveBundle`),
 * до появления настоящего бэка (Stage 6) это фронтовая генерация id, не
 * контракт. Два "встроенных" id — `"qa"`/`"doc"` — остаются как сид-данные
 * (`features/settings/mocks.ts#SERVICE_MOCKS`), но не как единственно
 * возможные значения. Публичная страница `/tariffs` (Stage 3) намеренно
 * работает только с этими двумя — см. `features/tariffs/types.ts#BuiltInServiceId`
 * (более узкий локальный алиас, не этот тип).
 */
export type ServiceId = string;

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
