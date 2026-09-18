import type { Locale } from '@/i18n/locale';

/** Numbers and currency always use Turkish separators, in both UI locales (plan D9). */
export const NUMBER_LOCALE = 'tr-TR';
export type Decimalish = string | number | null | undefined;
export type Unit = 'kWh' | 'MWh' | 'kW' | 'kVArh' | 'TRY' | 'tCO2e' | 'kgCO2e' | 'percent';
type Precision = { minFractionDigits?: number; maxFractionDigits?: number };

const EMPTY = '—';
const SYMBOLS: Record<Unit, string> = {
  kWh: 'kWh', MWh: 'MWh', kW: 'kW', kVArh: 'kVArh', TRY: '₺', tCO2e: 'tCO₂e', kgCO2e: 'kgCO₂e', percent: '%',
};

export function unitSymbol(u: Unit): string {
  return SYMBOLS[u];
}

// Decimal strings go to Intl as strings, so no float rounding touches them (plan D11).
function digits(v: string | number, o: Precision) {
  const own = typeof v === 'string' ? Math.min(20, (v.split('.')[1] ?? '').length) : 3;
  const min = o.minFractionDigits ?? 0;
  const max = o.maxFractionDigits ?? Math.max(own, min);
  return { minimumFractionDigits: Math.min(min, max), maximumFractionDigits: max };
}

export function formatNumber(v: Decimalish, o: Precision = {}): string {
  if (v === null || v === undefined || v === '') return EMPTY;
  return new Intl.NumberFormat(NUMBER_LOCALE, digits(v, o)).format(v as Parameters<Intl.NumberFormat['format']>[0]);
}

export function formatCurrency(v: Decimalish, o: Precision = {}): string {
  if (v === null || v === undefined || v === '') return EMPTY;
  return new Intl.NumberFormat(NUMBER_LOCALE, {
    style: 'currency',
    currency: 'TRY',
    minimumFractionDigits: o.minFractionDigits ?? 2,
    maximumFractionDigits: Math.max(o.maxFractionDigits ?? 2, o.minFractionDigits ?? 2),
  }).format(v as Parameters<Intl.NumberFormat['format']>[0]);
}

export function formatQuantity(v: Decimalish, u: Unit, o: Precision = {}): string {
  if (v === null || v === undefined || v === '') return EMPTY;
  if (u === 'TRY') return formatCurrency(v, o);
  if (u === 'percent') return `%${formatNumber(v, o)}`;
  return `${formatNumber(v, o)} ${SYMBOLS[u]}`;
}

const DATE_LOCALE: Record<Locale, string> = { tr: 'tr-TR', en: 'en-US' };

// A bare calendar date ('YYYY-MM-DD' or 'YYYY-MM') is pinned to noon UTC so Istanbul shows the same day (plan D19).
function toDate(iso: string): Date {
  const m = /^(\d{4})-(\d{2})(?:-(\d{2}))?$/.exec(iso);
  return m ? new Date(Date.UTC(+m[1], +m[2] - 1, m[3] ? +m[3] : 1, 12)) : new Date(iso);
}

export function formatDate(iso: string, locale: Locale): string {
  return new Intl.DateTimeFormat(DATE_LOCALE[locale], { dateStyle: 'medium', timeZone: 'Europe/Istanbul' }).format(
    toDate(iso),
  );
}

/** A timestamp with its time, always in Europe/Istanbul (R161). The alarm log
 *  and the Messages screen both need the minute, not just the day. */
export function formatDateTime(iso: string, locale: Locale): string {
  return new Intl.DateTimeFormat(DATE_LOCALE[locale], {
    dateStyle: 'medium',
    timeStyle: 'short',
    timeZone: 'Europe/Istanbul',
  }).format(toDate(iso));
}

export function formatMonth(iso: string, locale: Locale): string {
  return new Intl.DateTimeFormat(DATE_LOCALE[locale], {
    month: 'long',
    year: 'numeric',
    timeZone: 'Europe/Istanbul',
  }).format(toDate(iso));
}

/** File sizes in decimal units with Turkish separators: `1,5 MB`. */
export function formatBytes(bytes: number): string {
  const units = ['B', 'KB', 'MB', 'GB'];
  let value = bytes;
  let i = 0;
  while (value >= 1000 && i < units.length - 1) {
    value /= 1000;
    i++;
  }
  return `${formatNumber(Math.round(value * 10) / 10, { maxFractionDigits: 1 })} ${units[i]}`;
}
