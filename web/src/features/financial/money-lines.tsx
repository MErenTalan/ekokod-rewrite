'use client';

import { useTranslations } from 'next-intl';

import type { MoneyAmount } from '@/lib/api/types';
import { formatCurrency, formatNumber } from '@/lib/format';

export const money = (m: MoneyAmount) =>
  m.currency === 'TRY'
    ? formatCurrency(m.amount)
    : `${formatNumber(m.amount, { minFractionDigits: 2, maxFractionDigits: 2 })} ${m.currency}`;

/** Turkish possessive after a numeral ("9'u", "12'si"), by the numeral's last spoken word. */
export function coverageSuffix(n: number): string {
  const tens: Record<number, string> = {
    1: 'u',
    2: 'si',
    3: 'u',
    4: 'ı',
    5: 'si',
    6: 'ı',
    7: 'i',
    8: 'i',
    9: 'ı',
  };
  const ones: Record<number, string> = {
    1: 'i',
    2: 'si',
    3: 'ü',
    4: 'ü',
    5: 'i',
    6: 'sı',
    7: 'si',
    8: 'i',
    9: 'u',
  };
  if (n === 0) return 'ı';
  return n % 10 === 0 ? (n % 100 === 0 ? 'ü' : tens[(n / 10) % 10]) : ones[n % 10];
}

/** One line per currency (R253); an empty list is "veri yok", never 0. */
export function MoneyLines({ list, emptyClassName = 'type-small' }: { list: MoneyAmount[]; emptyClassName?: string }) {
  const t = useTranslations('financial');
  if (list.length === 0) return <span className={`text-foreground-muted ${emptyClassName}`}>{t('noData')}</span>;
  return (
    <span className="flex flex-col">
      {list.map((m) => (
        <span key={m.currency} className="type-data">
          {money(m)}
        </span>
      ))}
    </span>
  );
}
