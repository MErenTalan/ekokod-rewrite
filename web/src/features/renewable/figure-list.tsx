'use client';

import { useLocale, useTranslations } from 'next-intl';

import type { Locale } from '@/i18n/locale';
import {
  formatCurrency,
  formatDateTime,
  formatNumber,
  formatQuantity,
  type Unit,
} from '@/lib/format';

/** Message keys are camelCase; the API's fields and reasons are snake_case. */
export const reasonKey = (code: string) =>
  code.replace(/_([a-z0-9])/g, (_, c: string) => c.toUpperCase());

const OWN_UNITS = ['kg', 'km', 'trees', 'homes', 'years'] as const;
export type FigureUnit = Unit | (typeof OWN_UNITS)[number] | 'V' | 'Hz' | '';

export type Figure = {
  /** The API field name: it names the label and looks up the unavailable reason. */
  field: string;
  value?: string | number | null;
  format?: 'number' | 'money' | 'text' | 'plain' | 'datetime' | 'hour';
  unit?: FigureUnit;
  currency?: string | null;
  digits?: number;
};

/** A panel's figures as a definition list; a missing one says "veri yok" and why (R165, R292). */
export function FigureList({
  figures,
  unavailable,
}: {
  figures: Figure[];
  unavailable: Record<string, string>;
}) {
  const t = useTranslations('renewable');
  const locale = useLocale() as Locale;

  const shown = (f: Figure): string => {
    const v = f.value as string | number;
    switch (f.format) {
      case 'text':
        return t(`values.${reasonKey(String(v))}` as 'values.producing');
      case 'plain':
        return String(v);
      case 'datetime':
        return formatDateTime(String(v), locale);
      case 'hour':
        return t('units.hour', { hour: String(v).padStart(2, '0') });
      case 'money':
        return f.currency === 'TRY' || !f.currency
          ? formatCurrency(v, { maxFractionDigits: f.digits ?? 2 })
          : `${formatNumber(v, { maxFractionDigits: f.digits ?? 2 })} ${f.currency}`;
      default: {
        const precision = { maxFractionDigits: f.digits ?? 2 };
        if (!f.unit) return formatNumber(v, precision);
        if ((OWN_UNITS as readonly string[]).includes(f.unit))
          return `${formatNumber(v, precision)} ${t(`units.${f.unit}` as 'units.kg')}`;
        if (f.unit === 'V' || f.unit === 'Hz') return `${formatNumber(v, precision)} ${f.unit}`;
        return formatQuantity(v, f.unit as Unit, precision);
      }
    }
  };

  return (
    <dl className="grid gap-x-6 gap-y-3 sm:grid-cols-2">
      {figures.map((f) => {
        const missing = f.value === null || f.value === undefined || f.value === '';
        const reason = unavailable[f.field];
        return (
          <div key={f.field} className="flex min-w-0 flex-col gap-0.5">
            <dt className="text-foreground-muted type-caption">
              {t(`fields.${reasonKey(f.field)}` as 'fields.todayKwh')}
            </dt>
            {missing ? (
              <dd className="flex flex-col">
                <span className="text-foreground-muted type-body">{t('noData')}</span>
                {reason ? (
                  <span className="text-foreground-muted type-small">
                    {t(`reasons.${reasonKey(reason)}` as 'reasons.unknown')}
                  </span>
                ) : null}
              </dd>
            ) : (
              <dd className="text-foreground type-data [overflow-wrap:anywhere]">{shown(f)}</dd>
            )}
          </div>
        );
      })}
    </dl>
  );
}
