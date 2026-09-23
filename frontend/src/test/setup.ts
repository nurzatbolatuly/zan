import "@testing-library/jest-dom/vitest";

// jsdom не реализует matchMedia — нужен ThreadMessages.tsx (prefers-reduced-motion,
// Stage 5) и другим местам, которые проверяют media-query. Первый реальный
// потребитель в тестах — ChatPage.test.tsx (Stage 6).
// jsdom тоже не реализует scrollIntoView (ThreadMessages.tsx — автоскролл к
// новому сообщению) — тот же первый реальный потребитель, ChatPage.test.tsx.
if (typeof Element !== "undefined" && !Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {};
}

if (typeof window !== "undefined" && !window.matchMedia) {
  window.matchMedia = (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  });
}
