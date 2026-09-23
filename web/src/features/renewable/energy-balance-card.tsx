'use client';

import { useLocale, useTranslations } from 'next-intl';

import { LineChart } from '@/components/charts/line-chart';
import type { Granularity } from '@/components/domain/period-filter-bar';
import type { Locale } from '@/i18n/locale';
import type { EnergyBalance } from '@/lib/api/types';
import { formatPeriod } from '@/lib/format-period';

/** §7.8's energy balance from /energy-balance; the battery filter has nothing to filter. */
export function EnergyBalanceCard({
  data,
  granularity,
  loading,
}: {
  data?: EnergyBalance;
  granularity: Granularity;
  loading?: boolean;
}) {
  const t = useTranslations('renewable');
  const tc = useTranslations('consumption');
  const locale = useLocale() as Locale;
  const rows = (data?.items ?? []).map((r) => ({
    x: formatPeriod(r.period_start, granularity, locale),
    consumption: r.consumption ?? null,
    generation: r.generation ?? null,
    gridImport: r.grid_import ?? null,
    gridExport: r.grid_export ?? null,
  }));
  return (
    <LineChart
      title={t('panels.energyBalance')}
      description={t('panels.energyBalanceHint')}
      xLabel={t('production.period')}
      data={rows}
      series={[
        { key: 'consumption', label: t('balance.consumption'), kind: 'consumption', unit: 'kWh' },
        { key: 'generation', label: t('balance.generation'), kind: 'generation', unit: 'kWh' },
        { key: 'gridImport', label: t('balance.gridImport'), kind: 'cost', unit: 'kWh' },
        { key: 'gridExport', label: t('balance.gridExport'), kind: 'revenue', unit: 'kWh' },
      ]}
      height={240}
      loading={loading}
      footnote={t('panels.batteryUnavailable')}
      empty={{ title: t('production.empty'), description: tc('chart.emptyHint') }}
    />
  );
}
