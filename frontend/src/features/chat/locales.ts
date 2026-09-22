import type { Lang } from "@/shared/types/common";

/**
 * UI-строки экрана «Чат» (не сам демо-контент диалога — тот в mocks.ts).
 * Форма Record<Lang, ChatDictionary>, как договорено в instructions.md
 * («Конвенции» → i18n) — используется напрямую как `chatDictionary[lang]`.
 */
export interface ChatDictionary {
  emptyTitle: string;
  emptySubtitle: string;
  quickTopicsLabel: string;
  composerPlaceholder: string;
  attachLabel: string;
  removeAttachmentLabel: string;
  voiceStartLabel: string;
  voiceStopLabel: string;
  recordingHint: string;
  sendLabel: string;
  footerDisclaimer: string;
  assistantLabel: string;
  assistantTypingLabel: string;
  sourcesToggle: (count: number) => string;
  helpful: string;
  notHelpful: string;
  freeFollowup: string;
  downloadPdf: string;
  downloadDocx: string;
  editDocument: string;
  documentActionUnavailable: string;
  onboardingTitle: string;
  onboardingSubtitle: string;
  onboardingSteps: string[];
  onboardingDisclaimer: string;
  onboardingCta: string;
  payTitleConsultation: string;
  payDescriptionConsultation: string;
  payTitleDocument: string;
  payDescriptionDocument: string;
  payAmountField: string;
  payConfirm: string;
  payCancel: string;
  payNote: string;
}

export const chatDictionary: Record<Lang, ChatDictionary> = {
  ru: {
    emptyTitle: "Опишите свою ситуацию — объясним, что говорит закон",
    emptySubtitle:
      "Отвечаем простым языком и показываем, на каких статьях основан ответ.",
    quickTopicsLabel: "Быстрый старт",
    composerPlaceholder: "Опишите ситуацию своими словами…",
    attachLabel: "Прикрепить файл",
    removeAttachmentLabel: "Убрать вложение",
    voiceStartLabel: "Голосовой ввод",
    voiceStopLabel: "Готово",
    recordingHint: "Говорите — текст появится в поле ввода",
    sendLabel: "Отправить сообщение",
    footerDisclaimer: "Zan даёт справочную информацию и не заменяет юриста.",
    assistantLabel: "ZAN · ОТВЕТ",
    assistantTypingLabel: "ZAN печатает…",
    sourcesToggle: (count) => `Показать статьи закона (${count})`,
    helpful: "Полезно",
    notHelpful: "Не помогло",
    freeFollowup: "Уточнения — бесплатно",
    downloadPdf: "Скачать PDF",
    downloadDocx: "Скачать Word",
    editDocument: "Исправить",
    documentActionUnavailable: "Пока недоступно",
    onboardingTitle: "Понятные ответы по законам Казахстана",
    onboardingSubtitle:
      "Опишите ситуацию обычными словами — получите ответ и ссылки на статьи закона.",
    onboardingSteps: [
      "Опишите ситуацию своими словами — текстом, голосом или приложите документ.",
      "Оплата списывается один раз за обращение. Уточнять внутри него можно сколько угодно.",
      "Получите ответ со ссылками на статьи закона, при необходимости — готовый документ.",
    ],
    onboardingDisclaimer:
      "Zan — справочный сервис. Ответы основаны на действующем законодательстве РК, но не являются юридической консультацией и не заменяют обращение к адвокату по сложным делам.",
    onboardingCta: "Понятно, начать",
    payTitleConsultation: "Оплата консультации",
    payDescriptionConsultation:
      "Спишем один раз за это обращение. Уточнения внутри него — бесплатно.",
    payTitleDocument: "Оплата документа",
    payDescriptionDocument: "Спишем один раз за подготовку документа по этому обращению.",
    payAmountField: "К оплате",
    payConfirm: "Оплатить картой",
    payCancel: "Отмена",
    payNote: "Демонстрационная оплата — деньги не списываются.",
  },
  kz: {
    emptyTitle: "Жағдайыңызды жазыңыз — заң не дейтінін түсіндіреміз",
    emptySubtitle:
      "Қарапайым тілмен жауап береміз және негіз болған баптарды көрсетеміз.",
    quickTopicsLabel: "Жылдам бастау",
    composerPlaceholder: "Жағдайды өз сөзіңізбен жазыңыз…",
    attachLabel: "Файл тіркеу",
    removeAttachmentLabel: "Тіркемені алып тастау",
    voiceStartLabel: "Дауыстық енгізу",
    voiceStopLabel: "Дайын",
    recordingHint: "Сөйлеңіз — мәтін енгізу жолында пайда болады",
    sendLabel: "Хабарламаны жіберу",
    footerDisclaimer: "Zan анықтамалық ақпарат береді және заңгерді алмастырмайды.",
    assistantLabel: "ZAN · ЖАУАП",
    assistantTypingLabel: "ZAN жазып жатыр…",
    sourcesToggle: (count) => `Заң баптарын көрсету (${count})`,
    helpful: "Пайдалы",
    notHelpful: "Көмектеспеді",
    freeFollowup: "Нақтылау — тегін",
    downloadPdf: "PDF жүктеу",
    downloadDocx: "Word жүктеу",
    editDocument: "Түзету",
    documentActionUnavailable: "Әзірге қолжетімсіз",
    onboardingTitle: "Қазақстан заңдары бойынша түсінікті жауаптар",
    onboardingSubtitle:
      "Жағдайды қарапайым сөзбен жазыңыз — жауап пен заң баптарына сілтеме аласыз.",
    onboardingSteps: [
      "Жағдайды өз сөзіңізбен жазыңыз — мәтінмен, дауыспен немесе құжат тіркеңіз.",
      "Төлем бір өтініш үшін бір рет алынады. Оның ішінде қалағаныңызша нақтылаңыз.",
      "Заң баптарына сілтемесі бар жауап, қажет болса — дайын құжат аласыз.",
    ],
    onboardingDisclaimer:
      "Zan — анықтамалық қызмет. Жауаптар ҚР қолданыстағы заңнамасына негізделген, бірақ заң кеңесі болып саналмайды және күрделі істерде адвокатқа жүгінуді алмастырмайды.",
    onboardingCta: "Түсінікті, бастау",
    payTitleConsultation: "Кеңес ақысын төлеу",
    payDescriptionConsultation:
      "Осы өтініш үшін бір рет алынады. Ішіндегі нақтылаулар — тегін.",
    payTitleDocument: "Құжат ақысын төлеу",
    payDescriptionDocument: "Осы өтініш бойынша құжат дайындау үшін бір рет алынады.",
    payAmountField: "Төленетін сома",
    payConfirm: "Картамен төлеу",
    payCancel: "Болдырмау",
    payNote: "Демонстрациялық төлем — ақша алынбайды.",
  },
};
