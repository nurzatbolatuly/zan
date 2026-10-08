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
    tabTemplates: string;
    tabAnalytics: string;
    accessDeniedTitle: string;
    accessDeniedBody: string;
    save: string;
    cancel: string;
    /** Общая ошибка/повтор для всех вкладок (все читают `/admin/*`). */
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
  templates: {
    typesTitle: string;
    typesSub: string;
    addType: string;
    newTypeTitle: string;
    renameTypeTitle: string;
    fTypeName: string;
    typeNameRequiredError: string;
    typeSavedToast: string;
    typeDeleteConfirmTitle: string;
    typeDeleteConfirmMessage: string;
    typeDeletedToast: string;
    /** "3 шаблона" — подпись строки типа в справочнике. */
    templatesCount: (count: number) => string;
    templatesTitle: string;
    templatesSub: string;
    uploadTemplate: string;
    noTypesHint: string;
    emptyTitle: string;
    emptyDescription: string;
    newTemplateTitle: string;
    editTemplateTitle: string;
    fTitle: string;
    fType: string;
    fTypePlaceholder: string;
    fFile: string;
    chooseFile: string;
    replaceFile: string;
    /** maxSize — уже отформатированный лимит (`formatFileSize`, тот же вид, что в чате). */
    fileHint: (maxSize: string) => string;
    titleRequiredError: string;
    typeRequiredError: string;
    fileRequiredError: string;
    unsupportedFileError: string;
    fileTooLargeError: (maxSize: string) => string;
    /** Пока идёт сохранение: DOCX конвертируется в PDF на бэке синхронно. */
    savingNote: string;
    templateSavedToast: string;
    templateDeleteConfirmTitle: string;
    templateDeleteConfirmMessage: string;
    templateDeletedToast: string;
    viewAction: string;
    editAction: string;
    deleteAction: string;
    openInNewTab: string;
    close: string;
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

/** Подписи вкладки «Шаблоны» — передаются её компонентам одним пропом `labels`. */
export type TemplatesDictionary = SettingsDictionary["templates"];

// Формы declension — те же, что в features/tariffs/locales.ts (Stage 3), это
// один и тот же фиксированный набор из двух услуг (qa/doc), не два независимых источника правды.
const QA_FORMS_RU: readonly [string, string, string] = ["запрос", "запроса", "запросов"];
const DOC_FORMS_RU: readonly [string, string, string] = [
  "документ",
  "документа",
  "документов",
];
const TEMPLATE_FORMS_RU: readonly [string, string, string] = [
  "шаблон",
  "шаблона",
  "шаблонов",
];

// Строки — 1:1 из прототипа (Zan.dc.html:599-619 ru / 673-693 kz), плюс новые
// для того, чего в DSL-прототипе не было по-настоящему (тосты сохранения,
// сообщения валидации форм — прототип не валидировал ввод).
export const settingsDictionary: Record<Lang, SettingsDictionary> = {
  ru: {
    common: {
      title: "Настройки",
      subtitle:
        "Системные промпты агентов, тарифы и шаблоны документов. Раздел будет скрыт под роль администратора.",
      tabPrompts: "Промпты",
      tabTariffs: "Тарифы",
      tabTemplates: "Шаблоны",
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
    templates: {
      typesTitle: "Типы документов",
      typesSub: "Справочник, к которому относится каждый шаблон.",
      addType: "Добавить тип",
      newTypeTitle: "Новый тип документа",
      renameTypeTitle: "Переименовать тип",
      fTypeName: "НАЗВАНИЕ ТИПА",
      typeNameRequiredError: "Укажите название типа",
      typeSavedToast: "Тип документа сохранён",
      typeDeleteConfirmTitle: "Удалить тип документа?",
      typeDeleteConfirmMessage:
        "Тип исчезнет из справочника. Удалить можно только тип, у которого нет шаблонов.",
      typeDeletedToast: "Тип документа удалён",
      templatesCount: (count) => `${count} ${pluralRu(count, TEMPLATE_FORMS_RU)}`,
      templatesTitle: "Шаблоны документов",
      templatesSub: "Образцы в PDF или DOCX. Открываются как PDF — только для чтения.",
      uploadTemplate: "Загрузить шаблон",
      noTypesHint: "Сначала добавьте хотя бы один тип документа.",
      emptyTitle: "Шаблонов пока нет",
      emptyDescription:
        "Загрузите первый образец: договор, приказ, исковое заявление и т. д.",
      newTemplateTitle: "Новый шаблон",
      editTemplateTitle: "Изменить шаблон",
      fTitle: "НАЗВАНИЕ ШАБЛОНА",
      fType: "ТИП ДОКУМЕНТА",
      fTypePlaceholder: "Выберите тип",
      fFile: "ФАЙЛ",
      chooseFile: "Выбрать файл",
      replaceFile: "Заменить файл",
      fileHint: (maxSize) => `PDF или DOCX, до ${maxSize}`,
      titleRequiredError: "Укажите название шаблона",
      typeRequiredError: "Выберите тип документа",
      fileRequiredError: "Выберите файл",
      unsupportedFileError: "Подходят только файлы PDF и DOCX",
      fileTooLargeError: (maxSize) => `Файл больше ${maxSize}`,
      savingNote: "Сохраняем. DOCX переводится в PDF — это займёт несколько секунд.",
      templateSavedToast: "Шаблон сохранён",
      templateDeleteConfirmTitle: "Удалить шаблон?",
      templateDeleteConfirmMessage:
        "Шаблон и его файл будут удалены. Действие нельзя отменить.",
      templateDeletedToast: "Шаблон удалён",
      viewAction: "Открыть",
      editAction: "Изменить",
      deleteAction: "Удалить",
      openInNewTab: "Открыть в новой вкладке",
      close: "Закрыть",
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
        "Агенттердің жүйелік промпттары, тарифтер және құжат үлгілері. Бөлім әкімші рөліне жабылады.",
      tabPrompts: "Промпттар",
      tabTariffs: "Тарифтер",
      tabTemplates: "Үлгілер",
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
    templates: {
      typesTitle: "Құжат түрлері",
      typesSub: "Әр үлгі жататын анықтамалық.",
      addType: "Түр қосу",
      newTypeTitle: "Жаңа құжат түрі",
      renameTypeTitle: "Түрдің атауын өзгерту",
      fTypeName: "ТҮР АТАУЫ",
      typeNameRequiredError: "Түр атауын көрсетіңіз",
      typeSavedToast: "Құжат түрі сақталды",
      typeDeleteConfirmTitle: "Құжат түрін жою керек пе?",
      typeDeleteConfirmMessage:
        "Түр анықтамалықтан жойылады. Тек үлгісі жоқ түрді жоюға болады.",
      typeDeletedToast: "Құжат түрі жойылды",
      templatesCount: (count) => `${count} үлгі`,
      templatesTitle: "Құжат үлгілері",
      templatesSub: "PDF немесе DOCX үлгілері. PDF ретінде тек оқу үшін ашылады.",
      uploadTemplate: "Үлгі жүктеу",
      noTypesHint: "Алдымен кемінде бір құжат түрін қосыңыз.",
      emptyTitle: "Әзірге үлгілер жоқ",
      emptyDescription: "Алғашқы үлгіні жүктеңіз: шарт, бұйрық, талап арыз және т. б.",
      newTemplateTitle: "Жаңа үлгі",
      editTemplateTitle: "Үлгіні өзгерту",
      fTitle: "ҮЛГІ АТАУЫ",
      fType: "ҚҰЖАТ ТҮРІ",
      fTypePlaceholder: "Түрін таңдаңыз",
      fFile: "ФАЙЛ",
      chooseFile: "Файл таңдау",
      replaceFile: "Файлды ауыстыру",
      fileHint: (maxSize) => `PDF немесе DOCX, ең көбі ${maxSize}`,
      titleRequiredError: "Үлгі атауын көрсетіңіз",
      typeRequiredError: "Құжат түрін таңдаңыз",
      fileRequiredError: "Файлды таңдаңыз",
      unsupportedFileError: "Тек PDF және DOCX файлдары жарамды",
      fileTooLargeError: (maxSize) => `Файл ${maxSize} шегінен асады`,
      savingNote: "Сақталуда. DOCX PDF-ке айналдырылады — бұл бірнеше секунд алады.",
      templateSavedToast: "Үлгі сақталды",
      templateDeleteConfirmTitle: "Үлгіні жою керек пе?",
      templateDeleteConfirmMessage:
        "Үлгі және оның файлы жойылады. Бұл әрекетті болдырмау мүмкін емес.",
      templateDeletedToast: "Үлгі жойылды",
      viewAction: "Ашу",
      editAction: "Өзгерту",
      deleteAction: "Жою",
      openInNewTab: "Жаңа қойындыда ашу",
      close: "Жабу",
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
