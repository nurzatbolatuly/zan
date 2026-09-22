// Заглушка вместо явного toggle "Подготовить документ" (снят по решению
// от 2026-09-20 — реальное определение "нужен документ или нет" по контексту
// сообщения делает бэк, Stage 6, а не явный контрол на фронте, как было в
// brief 3.1/прототипе). Пока бэка нет, ключевые слова — грубая мок-эвристика,
// которая целиком заменяется, а не дополняется, когда появится настоящая
// классификация с бэка (FRONT_CODING_STANDARDS.md §3).
const RU_KEYWORDS = [
  "документ",
  "претензи",
  "заявлени",
  "жалоб",
  "иск",
  "составь",
  "подготовь",
  "оформ",
];
const KZ_KEYWORDS = ["құжат", "талап", "арыз", "шағым", "дайында"];

const DOCUMENT_INTENT_KEYWORDS = [...RU_KEYWORDS, ...KZ_KEYWORDS];

export function looksLikeDocumentRequest(text: string): boolean {
  const normalized = text.toLowerCase();
  return DOCUMENT_INTENT_KEYWORDS.some((keyword) => normalized.includes(keyword));
}
