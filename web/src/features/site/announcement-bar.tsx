import { ArrowRight } from 'lucide-react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

/** The strip above the header (01 §7.19). */
export function AnnouncementBar() {
  const t = useTranslations('site.announcement');
  return (
    <section aria-label={t('text')} className="bg-primary text-on-primary">
      <p className="mx-auto flex max-w-7xl flex-wrap items-center justify-center gap-x-2 gap-y-1 px-4 py-2 text-center type-small">
        <span>{t('text')}</span>
        <Link href="/bill-calculator" className="inline-flex items-center gap-1 font-semibold underline underline-offset-2 pointer-coarse:min-h-11">
          {t('link')}
          <ArrowRight aria-hidden className="size-3.5" />
        </Link>
      </p>
    </section>
  );
}
