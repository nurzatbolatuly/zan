import type { Lang } from "@/shared/types/common";
import { pluralRu } from "@/shared/lib/format";

/**
 * Строки уровня app-shell (шапка/нав/баланс) — не принадлежат ни одной фиче,
 * поэтому живут в shared, а не в локальном словаре конкретной фичи (FRONT_CODING_STANDARDS.md §1).
 */
interface NavDictionary {
  tagline: string;
  chat: string;
  history: string;
  tariffs: string;
  settings: string;
  langLabel: string;
  themeToggle: string;
  balance: string;
  consultationsLeft: (count: number) => string;
  /** Общая copy для `useUnsavedChangesStore#guard` — переход из шапки/таб-бара при несохранённой форме. */
  unsavedTitle: string;
  unsavedConfirm: string;
  unsavedCancel: string;
  /** aria-label нав-лендмарка — общий для Header и MobileNav (Stage 5 аудит: было захардкожено по-русски в обоих местах). */
  sectionsLabel: string;
  /** 404-страница (`app/NotFoundPage.tsx`) — общий app-shell экран, не принадлежит фиче. */
  notFoundTitle: string;
  notFoundBody: string;
  notFoundCta: string;
  /** `app/ErrorBoundary.tsx` — та же причина, что у 404: не принадлежит фиче. */
  errorTitle: string;
  errorBody: string;
  errorRetry: string;
}

export const navDictionary: Record<Lang, NavDictionary> = {
  ru: {
    tagline: "юридический ассистент · РК",
    chat: "Чат",
    history: "История",
    tariffs: "Тарифы",
    settings: "Настройки",
    langLabel: "Рус",
    themeToggle: "Тема",
    balance: "Баланс",
    consultationsLeft: (count) =>
      `${count} ${pluralRu(count, ["консультация", "консультации", "консультаций"])}`,
    unsavedTitle: "Несохранённые изменения",
    unsavedConfirm: "Уйти",
    unsavedCancel: "Остаться",
    sectionsLabel: "Разделы",
    notFoundTitle: "Страница не найдена",
    notFoundBody: "Такого раздела нет — возможно, ссылка устарела.",
    notFoundCta: "На главную",
    errorTitle: "Что-то пошло не так",
    errorBody:
      "Страница не смогла загрузиться. Попробуйте обновить — мы уже знаем об ошибке.",
    errorRetry: "Попробовать снова",
  },
  kz: {
    tagline: "заң көмекшісі · ҚР",
    chat: "Чат",
    history: "Тарих",
    tariffs: "Тарифтер",
    settings: "Баптаулар",
    langLabel: "Қаз",
    themeToggle: "Тема",
    balance: "Баланс",
    consultationsLeft: (count) => `${count} кеңес`,
    unsavedTitle: "Сақталмаған өзгерістер",
    unsavedConfirm: "Кету",
    unsavedCancel: "Қалу",
    sectionsLabel: "Бөлімдер",
    notFoundTitle: "Бет табылмады",
    notFoundBody: "Мұндай бөлім жоқ — сілтеме ескірген болуы мүмкін.",
    notFoundCta: "Басты бетке",
    errorTitle: "Бірдеңе дұрыс болмады",
    errorBody: "Бет жүктелмеді. Жаңартып көріңіз — біз қатені білеміз.",
    errorRetry: "Қайта көру",
  },
};
