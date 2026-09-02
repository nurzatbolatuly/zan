import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { isLocale, locales, type Locale } from '../../i18n/locales';
import { getDictionary } from '../../i18n/dictionary';
import { DictionaryProvider } from '../../i18n/DictionaryProvider';
import { LocaleSwitcher } from '../../components/LocaleSwitcher';

export function generateStaticParams() {
  return locales.map((locale) => ({ locale }));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string }>;
}): Promise<Metadata> {
  const { locale } = await params;
  if (!isLocale(locale)) return {};
  const dict = getDictionary(locale);
  return { title: dict.meta.title, description: dict.meta.description };
}

export default async function LocaleLayout({
  children,
  params,
}: {
  children: ReactNode;
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  if (!isLocale(locale)) notFound();
  const dict = getDictionary(locale as Locale);

  return (
    <DictionaryProvider locale={locale}>
      <div className="mx-auto flex min-h-screen max-w-2xl flex-col px-4 py-8">
        <header className="mb-8 flex items-start justify-between gap-4">
          <div>
            <Link href={`/${locale}`} className="text-xl font-semibold text-slate-900">
              zan
            </Link>
            <p className="mt-1 text-sm text-slate-500">{dict.nav.tagline}</p>
            <nav className="mt-3 flex gap-4 text-sm text-slate-600">
              <Link href={`/${locale}`} className="hover:text-slate-900">
                {dict.nav.home}
              </Link>
              <Link href={`/${locale}/history`} className="hover:text-slate-900">
                {dict.nav.history}
              </Link>
              <Link href={`/${locale}/analytics`} className="hover:text-slate-900">
                {dict.nav.analytics}
              </Link>
            </nav>
          </div>
          <LocaleSwitcher />
        </header>

        <main className="flex-1">{children}</main>

        <footer className="mt-12 border-t border-slate-200 pt-4 text-xs text-slate-500">
          {dict.footer.disclaimer}
        </footer>
      </div>
    </DictionaryProvider>
  );
}
