import type { Lang } from "@/shared/types/common";
import type { ReplyProgressStep } from "./replyProgress";

/**
 * UI-строки экрана «Чат» (не сам демо-контент диалога — тот в mocks.ts).
 * Форма Record<Lang, ChatDictionary>, как договорено в instructions.md
 * («Конвенции» → i18n) — используется напрямую как `chatDictionary[lang]`.
 */
export interface QuickTopic {
  label: string;
  draft: string;
}

export interface ChatDictionary {
  emptyTitle: string;
  emptySubtitle: string;
  quickTopicsLabel: string;
  quickTopics: QuickTopic[];
  composerPlaceholder: string;
  attachLabel: string;
  removeAttachmentLabel: string;
  fileTooLarge: (maxSizeLabel: string) => string;
  voiceStartLabel: string;
  voiceStopLabel: string;
  recordingHint: string;
  sendLabel: string;
  footerDisclaimer: string;
  assistantLabel: string;
  replyProgress: Record<ReplyProgressStep, string>;
  sourcesToggle: (count: number) => string;
  helpful: string;
  notHelpful: string;
  /** Тред `awaiting_payment`: вопрос сохранён, но не оплачен. */
  awaitingPaymentNotice: string;
  resumeQuestionAction: string;
  topUpBalanceAction: string;
  loadError: string;
  retry: string;
  /** Тред оплачен, но ответа нет — вызов LLM упал (internal_error,
   * кредит уже возвращён), показать это явно, а не молчать. */
  threadErrorNotice: string;
  threadCanceledNotice: string;
  onboardingTitle: string;
  onboardingSubtitle: string;
  onboardingSteps: string[];
  onboardingDisclaimer: string;
  onboardingCta: string;
  payTitleConsultation: string;
  payDescriptionConsultation: string;
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
    quickTopics: [
      {
        label: "Трудовой спор",
        draft: "Работодатель не выплатил зарплату вовремя. Что мне делать?",
      },
      {
        label: "Аренда жилья",
        draft: "Хочу проверить договор аренды квартиры перед подписанием.",
      },
      {
        label: "Возврат товара",
        draft: "Продавец отказывается принимать возврат бракованного товара.",
      },
      {
        label: "Штраф ГИБДД",
        draft: "Пришёл штраф за нарушение ПДД, с которым я не согласен.",
      },
    ],
    composerPlaceholder: "Опишите ситуацию…",
    attachLabel: "Прикрепить файл",
    removeAttachmentLabel: "Убрать вложение",
    fileTooLarge: (maxSizeLabel) => `Файл слишком большой — максимум ${maxSizeLabel}.`,
    voiceStartLabel: "Голосовой ввод",
    voiceStopLabel: "Готово",
    recordingHint: "Говорите — текст появится в поле ввода",
    sendLabel: "Отправить сообщение",
    footerDisclaimer: "Zan даёт справочную информацию и не заменяет юриста.",
    assistantLabel: "ZAN · ОТВЕТ",
    replyProgress: {
      sending: "Отправляем ваш вопрос…",
      readingFile: "Читаем приложенный документ…",
      analyzing: "Разбираемся в вашей ситуации…",
      checkingLaw: "Сверяемся с законодательством РК…",
      composing: "Готовим ответ…",
      takingLonger: "Вопрос непростой — нужно ещё немного времени…",
    },
    sourcesToggle: (count) => `Показать статьи закона (${count})`,
    helpful: "Полезно",
    notHelpful: "Не помогло",
    awaitingPaymentNotice:
      "Обращение ожидает оплаты. Пополните баланс в «Тарифах», затем перезапустите вопрос или задайте новый.",
    resumeQuestionAction: "Перезапустить вопрос",
    topUpBalanceAction: "Пополнить баланс",
    loadError: "Не удалось загрузить обращение.",
    retry: "Повторить",
    threadErrorNotice:
      "Не удалось обработать обращение — оплата возвращена на баланс. Начните новый тред.",
    threadCanceledNotice: "Обращение отменено.",
    onboardingTitle: "Понятные ответы по законам Казахстана",
    onboardingSubtitle:
      "Опишите ситуацию обычными словами — получите ответ и ссылки на статьи закона.",
    onboardingSteps: [
      "Опишите ситуацию своими словами — текстом, голосом или приложите документ.",
      "Один вопрос — одна консультация: каждый вопрос оплачивается отдельно.",
      "Получите ответ со ссылками на статьи закона, при необходимости — готовый документ.",
    ],
    onboardingDisclaimer:
      "Zan — справочный сервис. Ответы основаны на действующем законодательстве РК, но не являются юридической консультацией и не заменяют обращение к адвокату по сложным делам.",
    onboardingCta: "Понятно, начать",
    payTitleConsultation: "Оплата консультации",
    payDescriptionConsultation: "Спишем одну консультацию за этот вопрос.",
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
    quickTopics: [
      {
        label: "Еңбек дауы",
        draft: "Жұмыс беруші жалақыны уақытында төлемей отыр. Не істеуім керек?",
      },
      {
        label: "Тұрғын үй жалдау",
        draft: "Пәтер жалдау шартын қол қоюдан бұрын тексергім келеді.",
      },
      {
        label: "Тауарды қайтару",
        draft: "Сатушы ақаулы тауарды қайтарып алудан бас тартып отыр.",
      },
      {
        label: "ЖКО айыппұлы",
        draft: "Жол қозғалысы ережесін бұзғаны үшін келіспейтін айыппұл келді.",
      },
    ],
    composerPlaceholder: "Жағдайды өз сөзіңізбен жазыңыз…",
    attachLabel: "Файл тіркеу",
    removeAttachmentLabel: "Тіркемені алып тастау",
    fileTooLarge: (maxSizeLabel) => `Файл тым үлкен — ең көбі ${maxSizeLabel}.`,
    voiceStartLabel: "Дауыстық енгізу",
    voiceStopLabel: "Дайын",
    recordingHint: "Сөйлеңіз — мәтін енгізу жолында пайда болады",
    sendLabel: "Хабарламаны жіберу",
    footerDisclaimer: "Zan анықтамалық ақпарат береді және заңгерді алмастырмайды.",
    assistantLabel: "ZAN · ЖАУАП",
    replyProgress: {
      sending: "Сұрағыңызды жіберіп жатырмыз…",
      readingFile: "Тіркелген құжатты оқып жатырмыз…",
      analyzing: "Жағдайыңызды талдап жатырмыз…",
      checkingLaw: "ҚР заңнамасымен салыстырып жатырмыз…",
      composing: "Жауап дайындап жатырмыз…",
      takingLonger: "Сұрақ күрделі — тағы біраз уақыт қажет…",
    },
    sourcesToggle: (count) => `Заң баптарын көрсету (${count})`,
    helpful: "Пайдалы",
    notHelpful: "Көмектеспеді",
    awaitingPaymentNotice:
      "Өтініш төлемді күтуде. «Тарифтер» бөлімінде балансты толтырып, сұрақты қайта іске қосыңыз немесе жаңа сұрақ қойыңыз.",
    resumeQuestionAction: "Сұрақты қайта іске қосу",
    topUpBalanceAction: "Балансты толтыру",
    loadError: "Өтінімді жүктеу мүмкін болмады.",
    retry: "Қайталау",
    threadErrorNotice:
      "Өтінімді өңдеу сәтсіз аяқталды — төлем балансқа қайтарылды. Жаңа тред бастаңыз.",
    threadCanceledNotice: "Өтінім тоқтатылды.",
    onboardingTitle: "Қазақстан заңдары бойынша түсінікті жауаптар",
    onboardingSubtitle:
      "Жағдайды қарапайым сөзбен жазыңыз — жауап пен заң баптарына сілтеме аласыз.",
    onboardingSteps: [
      "Жағдайды өз сөзіңізбен жазыңыз — мәтінмен, дауыспен немесе құжат тіркеңіз.",
      "Бір сұрақ — бір кеңес: әр сұрақ бөлек төленеді.",
      "Заң баптарына сілтемесі бар жауап, қажет болса — дайын құжат аласыз.",
    ],
    onboardingDisclaimer:
      "Zan — анықтамалық қызмет. Жауаптар ҚР қолданыстағы заңнамасына негізделген, бірақ заң кеңесі болып саналмайды және күрделі істерде адвокатқа жүгінуді алмастырмайды.",
    onboardingCta: "Түсінікті, бастау",
    payTitleConsultation: "Кеңес ақысын төлеу",
    payDescriptionConsultation: "Осы сұрақ үшін бір кеңес алынады.",
    payAmountField: "Төленетін сома",
    payConfirm: "Картамен төлеу",
    payCancel: "Болдырмау",
    payNote: "Демонстрациялық төлем — ақша алынбайды.",
  },
};
