import { isLocale } from '../../i18n/locales';
import { getDictionary } from '../../i18n/dictionary';
import { notFound } from 'next/navigation';
import { RequestForm } from '../../components/RequestForm';

export default async function HomePage({ params }: { params: Promise<{ locale: string }> }) {
  const { locale } = await params;
  if (!isLocale(locale)) notFound();
  const dict = getDictionary(locale);

  return (
    <section>
      <h1 className="mb-2 text-2xl font-semibold text-slate-900">{dict.home.heading}</h1>
      <p className="mb-6 text-sm text-slate-600">{dict.home.subheading}</p>
      <RequestForm />
    </section>
  );
}
