'use client';

import { useTranslations } from 'next-intl';

import { BarChart } from '@/components/charts/bar-chart';
import type { FinancialMonthly } from '@/lib/api/types';

/** Consumption from invoices against plant production, month by month; months without data stay empty. */
export function YearlyChart({
  monthly,
  loading,
}: {
  monthly?: FinancialMonthly;
  loading?: boolean;
}) {
  const t = useTranslations('financial');
  const data = (monthly?.items ?? []).map((m) => ({
    x: t(`months.m${m.month}` as 'months.m1'),
    consumption: m.consumption_kwh ?? null,
    production: m.production_kwh ?? null,
  }));
  const hasData = data.some((d) => d.consumption !== null || d.production !== null);
  return (
    <BarChart
      title={t('chart.title')}
      description={t('chart.description')}
      xLabel={t('chart.month')}
      data={hasData ? data : []}
      series={[
        { key: 'consumption', label: t('chart.consumption'), kind: 'consumption', unit: 'kWh' },
        { key: 'production', label: t('chart.production'), kind: 'generation', unit: 'kWh' },
      ]}
      loading={loading}
      empty={{ title: t('chart.empty'), description: t('chart.emptyHint') }}
    />
  );
}
