import type { Lang } from "@/shared/types/common";
import type { AgentPrompt, AnalyticsSummary, Bundle, Service } from "./types";

/** Тексты промптов — 1:1 из прототипа (Zan.dc.html:780-789), моки до Stage 6 (`AgentPrompt` CRUD). */
export const AGENT_PROMPT_MOCKS: Record<Lang, AgentPrompt[]> = {
  ru: [
    {
      key: "qa",
      label: "Агент «Вопрос-ответ»",
      hint: "Системный промпт основного агента: тон, обязательные цитаты, ограничения.",
      value:
        "Ты — юридический ассистент по законодательству Республики Казахстан. Отвечай простым языком, без канцелярита, короткими абзацами. Всегда указывай конкретные статьи нормативных актов, на которых основан ответ. Если данных недостаточно — задай один уточняющий вопрос. Не давай гарантий исхода дела и рекомендуй обратиться к адвокату в сложных случаях.",
    },
    {
      key: "documents",
      label: "Агент «Работа с документами»",
      hint: "Промпт для анализа загруженных файлов и генерации документов.",
      value:
        "Ты анализируешь документы (договоры, решения, справки) по праву РК и готовишь новые документы: заявления, претензии, договоры. При анализе выделяй риски по пунктам с указанием номера пункта. При генерации соблюдай официальную структуру документа и оставляй поля для даты и подписи.",
    },
  ],
  kz: [
    {
      key: "qa",
      label: "«Сұрақ-жауап» агенті",
      hint: "Негізгі агенттің жүйелік промпты: тон, міндетті дәйексөздер, шектеулер.",
      value:
        "Сен — Қазақстан Республикасы заңнамасы бойынша заң көмекшісісің. Қарапайым тілмен, қысқа абзацтармен жауап бер. Жауаптың негізі болған нақты баптарды әрқашан көрсет. Дерек жеткіліксіз болса — бір нақтылау сұрағын қой. Іс нәтижесіне кепілдік берме, күрделі жағдайда адвокатқа жүгінуді ұсын.",
    },
    {
      key: "documents",
      label: "«Құжаттармен жұмыс» агенті",
      hint: "Жүктелген файлдарды талдау және құжат дайындау промпты.",
      value:
        "Сен ҚР құқығы бойынша құжаттарды (шарттар, шешімдер, анықтамалар) талдайсың және жаңа құжаттар дайындайсың: өтініштер, талаптар, шарттар. Талдау кезінде тәуекелдерді тармақ нөмірімен көрсет. Дайындау кезінде ресми құрылымды сақта, күні мен қолы үшін орын қалдыр.",
    },
  ],
};

/** Услуги — 1:1 из прототипа (Zan.dc.html:754-763), id пока мок (см. types.ts#ServiceId). */
export const SERVICE_MOCKS: Record<Lang, Service[]> = {
  ru: [
    { id: "qa", typeLabel: "Консультация", name: "Вопрос-ответ", unitPriceTenge: 2900 },
    {
      id: "doc",
      typeLabel: "Документ",
      name: "Подготовка документа",
      unitPriceTenge: 4900,
    },
  ],
  kz: [
    { id: "qa", typeLabel: "Кеңес", name: "Сұрақ-жауап", unitPriceTenge: 2900 },
    { id: "doc", typeLabel: "Құжат", name: "Құжат дайындау", unitPriceTenge: 4900 },
  ],
};

/** Тарифы (наборы услуг) — 1:1 из прототипа (Zan.dc.html:765-778). */
export const BUNDLE_MOCKS: Record<Lang, Bundle[]> = {
  ru: [
    {
      id: "b1",
      name: "1 вопрос",
      discountPercent: 0,
      items: [{ serviceId: "qa", qty: 1 }],
    },
    {
      id: "b2",
      name: "Пакет вопросов",
      discountPercent: 15,
      items: [{ serviceId: "qa", qty: 3 }],
    },
    {
      id: "b3",
      name: "Вопрос + документ",
      discountPercent: 10,
      items: [
        { serviceId: "qa", qty: 2 },
        { serviceId: "doc", qty: 1 },
      ],
    },
    {
      id: "b4",
      name: "Для бизнеса",
      discountPercent: 25,
      items: [
        { serviceId: "qa", qty: 10 },
        { serviceId: "doc", qty: 3 },
      ],
    },
  ],
  kz: [
    {
      id: "b1",
      name: "1 сұрақ",
      discountPercent: 0,
      items: [{ serviceId: "qa", qty: 1 }],
    },
    {
      id: "b2",
      name: "Сұрақ пакеті",
      discountPercent: 15,
      items: [{ serviceId: "qa", qty: 3 }],
    },
    {
      id: "b3",
      name: "Сұрақ + құжат",
      discountPercent: 10,
      items: [
        { serviceId: "qa", qty: 2 },
        { serviceId: "doc", qty: 1 },
      ],
    },
    {
      id: "b4",
      name: "Бизнес үшін",
      discountPercent: 25,
      items: [
        { serviceId: "qa", qty: 10 },
        { serviceId: "doc", qty: 3 },
      ],
    },
  ],
};

/**
 * Аналитика — счётчики 1:1 из прототипа (Zan.dc.html:949-968). Реальный
 * эндпоинт не детализирован в backend-roadmap.md (см. instructions.md
 * «Открытые вопросы») — мок до Stage 6.
 */
export const ANALYTICS_MOCKS: Record<Lang, AnalyticsSummary> = {
  ru: {
    totalRequests: 1020,
    avgResponseTimeLabel: "38 сек",
    satisfactionRateLabel: "92%",
    byStatus: [
      { status: "done", count: 812 },
      { status: "processing", count: 96 },
      { status: "clarify", count: 64 },
      { status: "queued", count: 21 },
      { status: "error", count: 18 },
      { status: "canceled", count: 9 },
    ],
  },
  kz: {
    totalRequests: 1020,
    avgResponseTimeLabel: "38 сек",
    satisfactionRateLabel: "92%",
    byStatus: [
      { status: "done", count: 812 },
      { status: "processing", count: 96 },
      { status: "clarify", count: 64 },
      { status: "queued", count: 21 },
      { status: "error", count: 18 },
      { status: "canceled", count: 9 },
    ],
  },
};
