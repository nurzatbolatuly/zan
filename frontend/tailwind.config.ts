import type { Config } from "tailwindcss";

/**
 * Значения здесь — реализация FRONT_DESIGN_SYSTEM.md, не источник истины.
 * Менять токен → сначала правка в FRONT_DESIGN_SYSTEM.md, потом здесь (см. §10 того файла).
 *
 * Breakpoints и spacing НЕ переопределены: дефолтная шкала Tailwind
 * (sm 640 / md 768 / lg 1024 / xl 1280, spacing 4px-шаг) совпадает
 * с FRONT_DESIGN_SYSTEM.md §3 и §7 один в один.
 *
 * darkMode не настроен нарочно: тема целиком на CSS-переменных
 * (shared/styles/tokens.css, [data-theme]) — bg-surface уже равен var(--surface),
 * которая сама меняется по теме. Tailwind-вариант `dark:` в компонентах не
 * используется нигде, поэтому не включаем — включённая, но не применяемая
 * настройка тоже мусор, который потом непонятно, трогать или нет.
 */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: "var(--bg)",
        surface: "var(--surface)",
        "surface-2": "var(--surface-2)",
        ink: "var(--ink)",
        muted: "var(--muted)",
        line: "var(--line)",
        accent: {
          DEFAULT: "var(--accent)",
          ink: "var(--accent-ink)",
          soft: "var(--accent-soft)",
        },
        warn: {
          DEFAULT: "var(--warn)",
          soft: "var(--warn-soft)",
        },
        danger: "var(--danger)",
      },
      fontFamily: {
        sans: ["Manrope", "system-ui", "sans-serif"],
        mono: ["IBM Plex Mono", "monospace"],
      },
      // FRONT_DESIGN_SYSTEM.md §2 — не добавлять размер мимо этой таблицы
      fontSize: {
        display: ["34px", { lineHeight: "1.15", letterSpacing: "-0.03em" }],
        h1: ["28px", { lineHeight: "1.2" }],
        h2: ["22px", { lineHeight: "1.25" }],
        h3: ["17px", { lineHeight: "1.3" }],
        "body-lg": ["17px", { lineHeight: "1.5" }],
        body: ["15px", { lineHeight: "1.5" }],
        "body-sm": ["14px", { lineHeight: "1.5" }],
        caption: ["13px", { lineHeight: "1.5" }],
        micro: ["11px", { lineHeight: "1.4", letterSpacing: "0.03em" }],
      },
      // FRONT_DESIGN_SYSTEM.md §4 — отличается от дефолтной шкалы Tailwind, задаётся явно
      borderRadius: {
        sm: "8px",
        md: "10px",
        lg: "12px",
        xl: "16px",
        "2xl": "20px",
        pill: "999px",
      },
      // FRONT_DESIGN_SYSTEM.md §5
      boxShadow: {
        card: "var(--shadow-card)",
        modal: "var(--shadow-modal)",
      },
      // FRONT_DESIGN_SYSTEM.md §6 — единственное место, откуда берутся z-index
      zIndex: {
        sticky: "20",
        composer: "15",
        overlay: "40",
        "overlay-high": "50",
        "overlay-top": "60",
        toast: "70",
      },
    },
  },
  plugins: [],
} satisfies Config;
