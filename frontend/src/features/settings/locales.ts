import type { Lang, ThreadStatus } from "@/shared/types/common";
import type { ServiceId } from "@/shared/types/tariff";
import { pluralRu } from "@/shared/lib/format";
import type { AgentPromptKey } from "./types";

interface SettingsDictionary {
  common: {
    title: string;
    subtitle: string;
    tabPrompts: string;
    tabTariffs: string;
    tabAnalytics: string;
    accessDeniedTitle: string;
    accessDeniedBody: string;
    save: string;
    cancel: string;
    /** Общая ошибка/повтор для всех трёх вкладок (Stage 6 — все читают `/admin/*`). */
    loadError: string;
    retry: string;
  };
  adminGate: {
    title: string;
    subtitle: string;
    tokenLabel: string;
    submit: string;
  };
  prompts: {
    title: string;
    saveButton: string;
    savedNote: string;
    unsavedGuardMessage: string;
    requiredError: string;
    /** label/hint — чистый UI-копирайт, бэк отдаёт только `agent_type`/`prompt_text`. */
    labels: Record<AgentPromptKey, { label: string; hint: string }>;
  };
  tariffs: {
    servicesTitle: string;
    servicesSub: string;
    fServiceType: string;
    fServiceName: string;
    fUnitPrice: string;
    serviceUpdatedToast: string;
    /**
     * "3 запроса" / "1 документ" — та же схема declension, что и в Stage 3
     * (features/tariffs/locales.ts#qtyLabel) для двух встроенных услуг
     * (qa/doc), казахский не склоняется. Для любой другой (созданной админом)
     * услуги склонение посчитать некому — берём `serviceName` как есть,
     * генерик-формат "{qty} × {name}" без declension (см. реализацию ниже).
     */
    qtyLabel: (serviceId: ServiceId, qty: number, serviceName: string) => string;
    decreaseQuantityLabel: (serviceName: string) => string;
    increaseQuantityLabel: (serviceName: string) => string;
    /** "−15%" — та же формула, что и у Stage 3 (features/tariffs/locales.ts#discountLabel). */
    discountLabel: (percent: number) => string;
    /** Каталог услуг фиксирован миграцией на бэке (Stage 6) — create/delete
     * услуги больше нет, есть только переключатель активности. */
    serviceActiveLabel: string;
    serviceInactiveLabel: string;
    serviceToggleActiveLabel: (serviceName: string, willBeActive: boolean) => string;
    bundlesTitle: string;
    bundlesSub: string;
    addTariff: string;
    editAction: string;
    deleteAction: string;
    deleteConfirmTitle: string;
    deleteConfirmMessage: string;
    deleteToastSuccess: string;
    newTariffTitle: string;
    editTariffTitle: string;
    fName: string;
    fDiscount: string;
    total: string;
    savedToast: string;
    nameRequiredError: string;
    atLeastOneItemError: string;
  };
  analytics: {
    title: string;
    subtitle: string;
    metricTotal: string;
    metricAvgTime: string;
    metricSatisfaction: string;
    /** `avg_processing_time_sec`/`satisfaction_rate` отсутствуют в ответе,
     * пока не накопилось данных (openapi.yaml — `omitempty`, не `0`). */
    noDataYet: string;
    byStatusTitle: string;
    statusLabel: Record<ThreadStatus, string>;
  };
}

// Формы declension — те же, что в features/tariffs/locales.ts (Stage 3), это
// один и тот же фиксированный набор из двух услуг (qa/doc), не два независимых источника правды.
const QA_FORMS_RU: readonly [string, string, string] = ["запрос", "запроса", "запросов"];
const DOC_FORMS_RU: readonly [string, string, string] = [
  "документ",
  "документа",
  "документов",
];

// Строки — 1:1 из прототипа (Zan.dc.html:599-619 ru / 673-693 kz), плюс новые
// для того, чего в DSL-прототипе не было по-настоящему (тосты сохранения,
// сообщения валидации форм — прототип не валидировал ввод).
export const settingsDictionary: Record<Lang, SettingsDictionary> = {
  ru: {
    common: {
      title: "Настройки",
      subtitle:
        "Системные промпты агентов и тарифы. Раздел будет скрыт под роль администратора.",
      tabPrompts: "Промпты",
      tabTariffs: "Тарифы",
      tabAnalytics: "Аналитика",
      accessDeniedTitle: "Доступ ограничен",
      accessDeniedBody: "Этот раздел доступен только администраторам.",
      save: "Сохранить",
      cancel: "Отмена",
      loadError: "Не удалось загрузить данные.",
      retry: "Повторить",
    },
    adminGate: {
      title: "Вход для администратора",
      subtitle: "Введите admin-токен, чтобы открыть настройки.",
      tokenLabel: "Admin-токен",
      submit: "Войти",
    },
    prompts: {
      title: "Промпты агентов",
      saveButton: "Сохранить",
      savedNote: "Сохранено",
      unsavedGuardMessage: "Изменения в промптах агентов не будут сохранены.",
      requiredError: "Промпт не может быть пустым",
      labels: {
        qa: {
          label: "Агент «Вопрос-ответ»",
          hint: "Системный промпт основного агента: тон, обязательные цитаты, ограничения.",
        },
        document: {
          label: "Агент «Работа с документами»",
          hint: "Промпт для анализа загруженных файлов и генерации документов.",
        },
      },
    },
    tariffs: {
      servicesTitle: "Услуги",
      servicesSub: "Базовые услуги и цена за одну единицу.",
      fServiceType: "ТИП УСЛУГИ",
      fServiceName: "НАЗВАНИЕ УСЛУГИ",
      fUnitPrice: "ЦЕНА, ₸",
      serviceUpdatedToast: "Услуга обновлена",
      qtyLabel: (serviceId, qty, serviceName) => {
        if (serviceId === "qa") return `${qty} ${pluralRu(qty, QA_FORMS_RU)}`;
        if (serviceId === "doc") return `${qty} ${pluralRu(qty, DOC_FORMS_RU)}`;
        return `${qty} × ${serviceName}`;
      },
      decreaseQuantityLabel: (serviceName) => `Уменьшить количество: ${serviceName}`,
      increaseQuantityLabel: (serviceName) => `Увеличить количество: ${serviceName}`,
      discountLabel: (percent) => `−${percent}%`,
      serviceActiveLabel: "Активна",
      serviceInactiveLabel: "Отключена",
      serviceToggleActiveLabel: (serviceName, willBeActive) =>
        `${willBeActive ? "Включить" : "Отключить"} услугу: ${serviceName}`,
      bundlesTitle: "Тарифы (наборы услуг)",
      bundlesSub: "Соберите тариф из услуг и, если нужно, дайте скидку.",
      addTariff: "Добавить тариф",
      editAction: "Изменить",
      deleteAction: "Удалить",
      deleteConfirmTitle: "Подтвердите удаление",
      deleteConfirmMessage:
        "Тариф будет удалён и больше не появится в списке. Действие нельзя отменить.",
      deleteToastSuccess: "Тариф удалён",
      newTariffTitle: "Новый тариф",
      editTariffTitle: "Изменить тариф",
      fName: "НАЗВАНИЕ ТАРИФА",
      fDiscount: "СКИДКА, %",
      total: "Сумма",
      savedToast: "Тариф сохранён",
      nameRequiredError: "Укажите название тарифа",
      atLeastOneItemError: "Добавьте хотя бы одну услугу в тариф",
    },
    analytics: {
      title: "Аналитика обращений",
      subtitle: "Данные показаны за последние 30 дней.",
      metricTotal: "Всего запросов",
      metricAvgTime: "Среднее время ответа",
      metricSatisfaction: "Доля «полезно»",
      noDataYet: "Пока нет данных",
      byStatusTitle: "По статусам",
      statusLabel: {
        awaiting_payment: "Ожидает оплаты",
        processing: "Обрабатывается",
        done: "Готово",
        error: "Ошибка",
        canceled: "Отменён",
      },
    },
  },
  kz: {
    common: {
      title: "Баптаулар",
      subtitle:
        "Агенттердің жүйелік промпттары және тарифтер. Бөлім әкімші рөліне жабылады.",
      tabPrompts: "Промпттар",
      tabTariffs: "Тарифтер",
      tabAnalytics: "Аналитика",
      accessDeniedTitle: "Қол жеткізу шектелген",
      accessDeniedBody: "Бұл бөлім тек әкімшілерге қолжетімді.",
      save: "Сақтау",
      cancel: "Болдырмау",
      loadError: "Деректерді жүктеу мүмкін болмады.",
      retry: "Қайталау",
    },
    adminGate: {
      title: "Әкімші кірісі",
      subtitle: "Баптауларды ашу үшін admin-токенді енгізіңіз.",
      tokenLabel: "Admin-токен",
      submit: "Кіру",
    },
    prompts: {
      title: "Агент промпттары",
      saveButton: "Сақтау",
      savedNote: "Сақталды",
      unsavedGuardMessage: "Агент промпттарындағы өзгерістер сақталмайды.",
      requiredError: "Промпт бос болмауы керек",
      labels: {
        qa: {
          label: "«Сұрақ-жауап» агенті",
          hint: "Негізгі агенттің жүйелік промпты: тон, міндетті дәйексөздер, шектеулер.",
        },
        document: {
          label: "«Құжаттармен жұмыс» агенті",
          hint: "Жүктелген файлдарды талдау және құжат дайындау промпты.",
        },
      },
    },
    tariffs: {
      servicesTitle: "Қызметтер",
      servicesSub: "Негізгі қызметтер және бір бірлік бағасы.",
      fServiceType: "ҚЫЗМЕТ ТҮРІ",
      fServiceName: "ҚЫЗМЕТ АТАУЫ",
      fUnitPrice: "БАҒАСЫ, ₸",
      serviceUpdatedToast: "Қызмет жаңартылды",
      qtyLabel: (serviceId, qty, serviceName) => {
        if (serviceId === "qa") return `${qty} сұраныс`;
        if (serviceId === "doc") return `${qty} құжат`;
        return `${qty} × ${serviceName}`;
      },
      decreaseQuantityLabel: (serviceName) => `Санын азайту: ${serviceName}`,
      increaseQuantityLabel: (serviceName) => `Санын көбейту: ${serviceName}`,
      discountLabel: (percent) => `−${percent}%`,
      serviceActiveLabel: "Белсенді",
      serviceInactiveLabel: "Өшірілген",
      serviceToggleActiveLabel: (serviceName, willBeActive) =>
        `Қызметті ${willBeActive ? "қосу" : "өшіру"}: ${serviceName}`,
      bundlesTitle: "Тарифтер (қызметтер жиынтығы)",
      bundlesSub: "Қызметтерден тариф құрастырыңыз, қажет болса жеңілдік беріңіз.",
      addTariff: "Тариф қосу",
      editAction: "Өзгерту",
      deleteAction: "Жою",
      deleteConfirmTitle: "Жоюды растаңыз",
      deleteConfirmMessage:
        "Тариф жойылады және тізімде көрінбейді. Бұл әрекетті болдырмау мүмкін емес.",
      deleteToastSuccess: "Тариф жойылды",
      newTariffTitle: "Жаңа тариф",
      editTariffTitle: "Тарифті өзгерту",
      fName: "ТАРИФ АТАУЫ",
      fDiscount: "ЖЕҢІЛДІК, %",
      total: "Сома",
      savedToast: "Тариф сақталды",
      nameRequiredError: "Тариф атауын көрсетіңіз",
      atLeastOneItemError: "Тарифке кемінде бір қызмет қосыңыз",
    },
    analytics: {
      title: "Өтініштер аналитикасы",
      subtitle: "Деректер соңғы 30 күн бойынша көрсетілген.",
      metricTotal: "Барлық сұраныстар",
      metricAvgTime: "Орташа жауап уақыты",
      metricSatisfaction: "«Пайдалы» үлесі",
      noDataYet: "Деректер әлі жоқ",
      byStatusTitle: "Статустар бойынша",
      statusLabel: {
        awaiting_payment: "Төлемді күтуде",
        processing: "Өңделуде",
        done: "Дайын",
        error: "Қате",
        canceled: "Тоқтатылған",
      },
    },
  },
};
