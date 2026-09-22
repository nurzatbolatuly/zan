import { describe, expect, it } from "vitest";
import { looksLikeDocumentRequest } from "./documentIntent";

describe("looksLikeDocumentRequest", () => {
  it.each([
    "Подготовьте претензию работодателю",
    "Нужен документ о расторжении договора",
    "Составьте заявление в инспекцию труда",
    "ДАЙЫНДАҢЫЗ талап хат",
  ])("распознаёт запрос на документ: %s", (text) => {
    expect(looksLikeDocumentRequest(text)).toBe(true);
  });

  it.each([
    "Работодатель не выплатил зарплату вовремя. Что мне делать?",
    "Хочу проверить договор аренды квартиры перед подписанием.",
    "Продавец отказывается принимать возврат бракованного товара.",
    "Пришёл штраф за нарушение ПДД, с которым я не согласен.",
  ])("не путает обычный вопрос с запросом документа: %s", (text) => {
    expect(looksLikeDocumentRequest(text)).toBe(false);
  });
});
