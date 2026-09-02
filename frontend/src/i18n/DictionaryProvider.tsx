'use client';

import { createContext, useContext, useMemo, type ReactNode } from 'react';
import { getDictionary, type Dictionary } from './dictionary';
import type { Locale } from './locales';

interface I18nContextValue {
  locale: Locale;
  dict: Dictionary;
}

const I18nContext = createContext<I18nContextValue | null>(null);

/**
 * Принимает только `locale` (сериализуемая строка) и сама вычисляет словарь клиентски —
 * Dictionary содержит функции (minLengthHint, download), а React Server Components не может
 * передать функцию как проп из серверного [locale]/layout.tsx в клиентский компонент. Словарь
 * маленький и синхронный, пересчитать его на клиенте дешевле, чем городить сериализацию.
 */
export function DictionaryProvider({ locale, children }: { locale: Locale; children: ReactNode }) {
  const value = useMemo<I18nContextValue>(
    () => ({ locale, dict: getDictionary(locale) }),
    [locale],
  );
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

function useI18n(): I18nContextValue {
  const value = useContext(I18nContext);
  if (!value) {
    throw new Error('useDictionary/useLocale должны использоваться внутри DictionaryProvider');
  }
  return value;
}

export function useDictionary(): Dictionary {
  return useI18n().dict;
}

export function useLocale(): Locale {
  return useI18n().locale;
}
