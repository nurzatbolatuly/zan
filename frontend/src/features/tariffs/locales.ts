import type { Lang } from "@/shared/types/common";
import { formatTenge, pluralRu } from "@/shared/lib/format";
import type { BuiltInServiceId } from "./types";

const QA_FORMS_RU: readonly [string, string, string] = ["запрос", "запроса", "запросов"];
const DOC_FORMS_RU: readonly [string, string, string] = [
  "документ",
  "документа",
  "документов",
];

/**
 * UI-строки экрана «Тарифы» (не сами цены/состав пакетов — те в mocks.ts/types.ts).
 * Форма Record<Lang, TariffsDictionary>, как договорено в instructions.md («Конвенции» → i18n).
 */
export interface TariffsDictionary {
  title: string;
  subtitle: string;
  buy: string;
  cancel: string;
  serviceName: Record<BuiltInServiceId, string>;
  /** "3 запроса" / "1 документ" — казахский не склоняется (форма одна на любое количество). */
  qtyLabel: (serviceId: BuiltInServiceId, qty: number) => string;
  /** "2 900 ₸ за 1 запрос" — цена за единицу для строки в своём наборе. */
  unitPriceLabel: (serviceId: BuiltInServiceId, priceTenge: number) => string;
  discountLabel: (percent: number) => string;
  bundleName: Record<string, string>;
  customTitle: string;
  customSubtitle: string;
  customCta: string;
  customModalTitle: string;
  customModalSubtitle: string;
  customTotalLabel: string;
  /** Кнопка своего набора не платит сразу — открывает общую PaymentModal (M3, PLAN.md §1). */
  customContinueCta: string;
  decreaseQuantityLabel: (serviceName: string) => string;
  increaseQuantityLabel: (serviceName: string) => string;
  payTitleBundle: (bundleName: string) => string;
  payDescriptionBundle: string;
  payTitleCustom: string;
  payDescriptionCustom: string;
  payAmountField: string;
  payConfirm: string;
  payCancel: string;
  payNote: string;
  purchaseSuccessToast: (label: string) => string;
}

export const tariffsDictionary: Record<Lang, TariffsDictionary> = {
  ru: {
    title: "Тарифы",
    subtitle:
      "Готовые пакеты для вопросов и для документов — или свой набор с точным количеством.",
    buy: "Купить",
    cancel: "Отмена",
    serviceName: { qa: "Вопрос-ответ", doc: "Подготовка документа" },
    qtyLabel: (serviceId, qty) =>
      `${qty} ${pluralRu(qty, serviceId === "qa" ? QA_FORMS_RU : DOC_FORMS_RU)}`,
    unitPriceLabel: (serviceId, priceTenge) =>
      `${formatTenge(priceTenge)} за 1 ${serviceId === "qa" ? QA_FORMS_RU[0] : DOC_FORMS_RU[0]}`,
    discountLabel: (percent) => `−${percent}%`,
    bundleName: {
      b1: "1 вопрос",
      b2: "Пакет вопросов",
      b3: "Вопрос + документ",
      b4: "Для бизнеса",
    },
    customTitle: "Свой набор запросов",
    customSubtitle: "Укажите нужное количество вопросов и документов вручную.",
    customCta: "Выбрать количество",
    customModalTitle: "Свой набор запросов",
    customModalSubtitle:
      "Укажите, сколько вопросов и документов нужно, и оплатите картой.",
    customTotalLabel: "К оплате",
    customContinueCta: "Продолжить к оплате",
    decreaseQuantityLabel: (serviceName) => `Уменьшить количество: ${serviceName}`,
    increaseQuantityLabel: (serviceName) => `Увеличить количество: ${serviceName}`,
    payTitleBundle: (bundleName) => `Оплата тарифа «${bundleName}»`,
    payDescriptionBundle:
      "Спишем один раз при покупке пакета. Пакет пополнит баланс консультаций.",
    payTitleCustom: "Оплата своего набора",
    payDescriptionCustom:
      "Спишем один раз за выбранное количество вопросов и документов.",
    payAmountField: "К оплате",
    payConfirm: "Оплатить картой",
    payCancel: "Отмена",
    payNote: "Демонстрационная оплата — деньги не списываются.",
    purchaseSuccessToast: (label) => `«${label}» активирован`,
  },
  kz: {
    title: "Тарифтер",
    subtitle:
      "Сұрақ пен құжат үшін дайын пакеттер — немесе өз санын дәл көрсетіп таңдаңыз.",
    buy: "Сатып алу",
    cancel: "Болдырмау",
    serviceName: { qa: "Сұрақ-жауап", doc: "Құжат дайындау" },
    qtyLabel: (serviceId, qty) => `${qty} ${serviceId === "qa" ? "сұраныс" : "құжат"}`,
    unitPriceLabel: (serviceId, priceTenge) =>
      `1 ${serviceId === "qa" ? "сұраныс" : "құжат"} — ${formatTenge(priceTenge)}`,
    discountLabel: (percent) => `−${percent}%`,
    bundleName: {
      b1: "1 сұрақ",
      b2: "Сұрақ пакеті",
      b3: "Сұрақ + құжат",
      b4: "Бизнес үшін",
    },
    customTitle: "Өз санын таңдау",
    customSubtitle: "Қажетті сұрақ пен құжат санын өзіңіз көрсетіңіз.",
    customCta: "Санын таңдау",
    customModalTitle: "Өз санын таңдау",
    customModalSubtitle: "Қанша сұрақ және құжат керегін көрсетіп, картамен төлеңіз.",
    customTotalLabel: "Төленетін сома",
    customContinueCta: "Төлеуге өту",
    decreaseQuantityLabel: (serviceName) => `Санын азайту: ${serviceName}`,
    increaseQuantityLabel: (serviceName) => `Санын көбейту: ${serviceName}`,
    payTitleBundle: (bundleName) => `«${bundleName}» тарифін төлеу`,
    payDescriptionBundle:
      "Пакетті сатып алғанда бір рет алынады. Пакет кеңес балансын толықтырады.",
    payTitleCustom: "Өз жиынтығыңызды төлеу",
    payDescriptionCustom: "Таңдалған сұрақ пен құжат саны үшін бір рет алынады.",
    payAmountField: "Төленетін сома",
    payConfirm: "Картамен төлеу",
    payCancel: "Болдырмау",
    payNote: "Демонстрациялық төлем — ақша алынбайды.",
    purchaseSuccessToast: (label) => `«${label}» іске қосылды`,
  },
};
