import type { Granularity } from '@/components/domain/period-filter-bar';
import type { Locale } from '@/i18n/locale';
import { formatDate, formatMonth } from '@/lib/format';

const LOCALES: Record<Locale, string> = { tr: 'tr-TR', en: 'en-US' };

/**
 * A period label for the granularity it was read at (R206): an hour keeps its
 * time, a day is a date, a month is its name and a year is its number.
 */
export function formatPeriod(periodStart: string, granularity: Granularity, locale: Locale): string {
  const iso = periodStart.slice(0, 10);
  switch (granularity) {
    case 'hourly':
      return `${formatDate(iso, locale)} ${new Intl.DateTimeFormat(LOCALES[locale], {
        hour: '2-digit',
        minute: '2-digit',
        timeZone: 'Europe/Istanbul',
        hourCycle: 'h23',
      }).format(new Date(periodStart))}`;
    case 'monthly':
      return formatMonth(iso.slice(0, 7), locale);
    case 'yearly':
      return iso.slice(0, 4);
    default:
      return formatDate(iso, locale);
  }
}
