import type { Lang, ThreadStatus } from "@/shared/types/common";
import type { HistoryPeriodFilter } from "./types";

interface HistoryDictionary {
  title: string;
  subtitle: string;
  searchPlaceholder: string;
  searchLabel: string;
  filterLabel: string;
  filterAll: string;
  statusLabel: Record<ThreadStatus, string>;
  periodFilterLabel: string;
  periodLabel: Record<HistoryPeriodFilter, string>;
  open: string;
  deleteAction: string;
  deleteConfirmTitle: string;
  deleteConfirmMessage: string;
  deleteToastSuccess: string;
  emptyNoThreadsTitle: string;
  emptyNoThreadsBody: string;
  emptyNoResultsTitle: string;
  emptyNoResultsBody: string;
  resetFilters: string;
  askQuestion: string;
  cancel: string;
}

// Строки — 1:1 из прототипа (Zan.dc.html:583-608, 629-630, 657-682, 704),
// набор статусов расширен с 4 до 6 по PLAN.md §5 (Stage 2 требует все
// ThreadStatus в фильтре, прототип показывал только 4 как демо-подмножество).
// Фильтр по периоду (periodLabel) — gap-add поверх прототипа, там его нет.
export const historyDictionary: Record<Lang, HistoryDictionary> = {
  ru: {
    title: "История",
    subtitle: "Обращения хранятся здесь. Откройте тред, чтобы продолжить разговор.",
    searchPlaceholder: "Поиск по обращениям",
    searchLabel: "Поиск по обращениям",
    filterLabel: "Статус",
    filterAll: "Все",
    statusLabel: {
      queued: "В очереди",
      processing: "Обрабатывается",
      clarify: "Ждёт уточнения",
      done: "Готово",
      error: "Ошибка",
      canceled: "Отменён",
    },
    periodFilterLabel: "Период",
    periodLabel: {
      all: "За всё время",
      today: "Сегодня",
      yesterday: "Вчера",
      "7d": "7 дней",
      "30d": "30 дней",
    },
    open: "Открыть тред",
    deleteAction: "Удалить",
    deleteConfirmTitle: "Подтвердите удаление",
    deleteConfirmMessage:
      "Обращение и его переписка будут удалены без возможности восстановления.",
    deleteToastSuccess: "Обращение удалено",
    emptyNoThreadsTitle: "Пока нет обращений",
    emptyNoThreadsBody: "Здесь появятся ваши консультации — с ответами и документами.",
    emptyNoResultsTitle: "Ничего не найдено",
    emptyNoResultsBody: "Попробуйте изменить запрос или сбросить фильтры.",
    resetFilters: "Сбросить фильтры",
    askQuestion: "Задать вопрос",
    cancel: "Отмена",
  },
  kz: {
    title: "Тарих",
    subtitle: "Өтініштер осында сақталады. Әңгімені жалғастыру үшін тредті ашыңыз.",
    searchPlaceholder: "Өтініштерден іздеу",
    searchLabel: "Өтініштерден іздеу",
    filterLabel: "Мәртебе",
    filterAll: "Барлығы",
    statusLabel: {
      queued: "Кезекте",
      processing: "Өңделуде",
      clarify: "Нақтылауды күтуде",
      done: "Дайын",
      error: "Қате",
      canceled: "Тоқтатылған",
    },
    periodFilterLabel: "Кезең",
    periodLabel: {
      all: "Барлық уақыт",
      today: "Бүгін",
      yesterday: "Кеше",
      "7d": "7 күн",
      "30d": "30 күн",
    },
    open: "Тредті ашу",
    deleteAction: "Жою",
    deleteConfirmTitle: "Жоюды растаңыз",
    deleteConfirmMessage: "Өтініш пен оның хат-хабары қайтарылмастай жойылады.",
    deleteToastSuccess: "Өтініш жойылды",
    emptyNoThreadsTitle: "Әзірге өтініш жоқ",
    emptyNoThreadsBody:
      "Мұнда сіздің кеңестеріңіз — жауаптар мен құжаттармен бірге көрінеді.",
    emptyNoResultsTitle: "Ештеңе табылмады",
    emptyNoResultsBody: "Сұранысты өзгертіп көріңіз немесе сүзгілерді алып тастаңыз.",
    resetFilters: "Сүзгілерді алып тастау",
    askQuestion: "Сұрақ қою",
    cancel: "Болдырмау",
  },
};
