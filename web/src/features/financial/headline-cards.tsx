'use client';

import { useTranslations } from 'next-intl';
import type { ReactNode } from 'react';

import type { FinancialSummary } from '@/lib/api/types';
import { formatNumber } from '@/lib/format';

import { coverageSuffix, MoneyLines } from './money-lines';

function Tile({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-2 rounded-lg border border-border bg-surface-raised p-4 in-data-[card=shadow]:border-card-edge in-data-[card=shadow]:shadow-md">
      <p className="text-foreground-muted type-caption">{label}</p>
      <div className="text-foreground type-metric [overflow-wrap:anywhere]">{children}</div>
    </div>
  );
}

/** R294's headline: counts, the period's figures per currency, and how many months had invoices. */
export function HeadlineCards({ summary }: { summary: FinancialSummary }) {
  const t = useTranslations('financial');
  const f = summary.figures;
  const kwh = (v?: string | null) => (v ? `${formatNumber(v, { maxFractionDigits: 2 })}` : null);
  const orNone = (v: string | null) =>
    v ?? <span className="text-foreground-muted type-body-lg">{t('noData')}</span>;
  return (
    <section className="flex flex-col gap-3">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <Tile label={t('headline.analyzers')}>{summary.analyzer_count}</Tile>
        <Tile label={t('headline.plants')}>{summary.plant_count}</Tile>
        <Tile label={`${t('headline.consumption')} (kWh)`}>{orNone(kwh(f.consumption_kwh))}</Tile>
        <Tile label={`${t('headline.production')} (kWh)`}>{orNone(kwh(f.production_kwh))}</Tile>
        <Tile label={t('headline.cost')}>
          <MoneyLines list={f.cost} />
        </Tile>
        <Tile label={t('headline.revenue')}>
          <MoneyLines list={f.revenue} />
        </Tile>
        <Tile label={t('headline.net')}>
          <MoneyLines list={f.net} />
        </Tile>
      </div>
      <p className="text-foreground-muted type-small">
        {summary.of === 1 && summary.with_data === 0
          ? t('headline.coverageMonth')
          : t('headline.coverage', {
              of: summary.of,
              withData: summary.with_data,
              suffix: coverageSuffix(summary.with_data),
            })}
      </p>
      {f.revenue_partial ? (
        <p className="text-warning type-small">{t('headline.partial')}</p>
      ) : null}
    </section>
  );
}
