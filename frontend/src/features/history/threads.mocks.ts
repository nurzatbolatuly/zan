import type { Thread } from "./types";

/**
 * `updatedAt` — относительно момента загрузки модуля (всегда в прошлом), не
 * захардкоженные даты прототипа: History теперь фильтруется по дням/
 * промежуткам (Сегодня/Вчера/7 дней/30 дней), и с фиксированными датами демо
 * через пару недель показывало бы пустой результат на "Сегодня"/"7 дней".
 * Смещения — в часах от "сейчас" (не `setUTCHours` на конкретный час: так
 * `updatedAt` не может случайно оказаться в будущем относительно `now` при
 * фильтрации, см. `filterThreads.ts`). Подобраны так, чтобы покрыть все
 * периоды сразу.
 */
function hoursAgoIso(hours: number): string {
  return new Date(Date.now() - hours * 60 * 60 * 1000).toISOString();
}

/**
 * Моки до Stage 6 (реальный `/threads`) — по одному треду на каждый
 * `ThreadStatus`, тексты 1:1 из прототипа (Zan.dc.html:735-740).
 */
export const THREAD_MOCKS: Thread[] = [
  {
    id: "thread-done-1",
    title: "Работодатель не отдаёт трудовую книжку",
    preview: "Претензия готова, можно скачать в PDF или Word",
    status: "done",
    updatedAt: hoursAgoIso(2),
    processingTimeSeconds: 622,
  },
  {
    id: "thread-clarify-1",
    title: "Проверка договора аренды помещения",
    preview: "Нужен скан второй страницы договора — без него не видно условия залога",
    status: "clarify",
    updatedAt: hoursAgoIso(26),
    processingTimeSeconds: 48,
  },
  {
    id: "thread-processing-1",
    title: "Регистрация ИП: какой налоговый режим выбрать",
    preview: "Ассистент готовит ответ…",
    status: "processing",
    updatedAt: hoursAgoIso(1),
    processingTimeSeconds: 26,
  },
  {
    id: "thread-queued-1",
    title: "Раздел имущества при разводе",
    preview: "Запрос в очереди, обычно занимает до 2 минут",
    status: "queued",
    updatedAt: hoursAgoIso(50),
    processingTimeSeconds: 0,
  },
  {
    id: "thread-error-1",
    title: "Штраф за просрочку уплаты налога",
    preview: "Не удалось обработать запрос — оплата возвращена на баланс",
    status: "error",
    updatedAt: hoursAgoIso(10 * 24 + 2),
    processingTimeSeconds: 143,
  },
  {
    id: "thread-canceled-1",
    title: "Оформление сотрудника-иностранца",
    preview: "Обращение отменено до оплаты",
    status: "canceled",
    updatedAt: hoursAgoIso(25 * 24 + 4),
    processingTimeSeconds: 4,
  },
];
