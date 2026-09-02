import type { ReactNode } from 'react';
import './globals.css';

/**
 * Корневой layout Next.js App Router — единственное место, где допустимы <html>/<body>
 * (см. app/[locale]/layout.tsx, который их НЕ дублирует). `lang` статично на "ru": сам
 * `<html>` рендерится до того, как известна локаль (она — параметр вложенного сегмента), а
 * не глобальная настройка страницы; локаль-специфичный текст всё равно приходит из словаря
 * в app/[locale]/layout.tsx.
 */
export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="ru">
      <body className="min-h-screen bg-slate-50 text-slate-900 antialiased">{children}</body>
    </html>
  );
}
