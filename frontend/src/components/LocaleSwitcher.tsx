'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { locales } from '../i18n/locales';
import { useDictionary, useLocale } from '../i18n/DictionaryProvider';

/** Меняет только первый сегмент пути (локаль), остальной путь (например, /requests/{id}) сохраняется. */
function replaceLocaleInPath(pathname: string, nextLocale: string): string {
  const segments = pathname.split('/');
  segments[1] = nextLocale;
  return segments.join('/') || '/';
}

export function LocaleSwitcher() {
  const pathname = usePathname();
  const currentLocale = useLocale();
  const dict = useDictionary();

  return (
    <nav aria-label="language" className="flex items-center gap-1 text-xs font-medium">
      {locales.map((locale, index) => (
        <span key={locale} className="flex items-center gap-1">
          {index > 0 && <span className="text-slate-300">/</span>}
          <Link
            href={replaceLocaleInPath(pathname, locale)}
            aria-current={locale === currentLocale ? 'true' : undefined}
            className={
              locale === currentLocale ? 'text-slate-900' : 'text-slate-400 hover:text-slate-700'
            }
          >
            {dict.localeSwitcher[locale]}
          </Link>
        </span>
      ))}
    </nav>
  );
}
