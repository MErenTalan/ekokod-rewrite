'use client';

import { FlaskConical } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';

import type { Locale } from '@/i18n/locale';
import { formatTime } from '@/lib/format';

/**
 * Tells a demo visitor that the data is sample data and when it returns to the baseline (R552). A status, not
 * an alert: it sits on every page and must not interrupt a screen reader each time. No dismiss.
 */
export function DemoBanner({ resetsAt }: { resetsAt?: string | null }) {
  const t = useTranslations('shell.demo.banner');
  const locale = useLocale() as Locale;
  if (!resetsAt) return null;
  return (
    <div
      role="status"
      data-demo-banner
      className="flex flex-wrap items-start gap-3 rounded-md border border-warning bg-warning-subtle p-3 text-foreground"
    >
      <FlaskConical aria-hidden className="mt-0.5 size-5 shrink-0 text-warning" />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <p className="font-semibold type-body">{t('title')}</p>
        <p className="type-small">{t('body')}</p>
      </div>
      <p className="shrink-0 font-semibold type-small">{t('next', { time: formatTime(resetsAt, locale) })}</p>
    </div>
  );
}
