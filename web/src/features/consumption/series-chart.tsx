'use client';

import { useLocale, useTranslations } from 'next-intl';
import { useMemo } from 'react';

import { LineChart } from '@/components/charts/line-chart';
import type { ChartSeries, Datum } from '@/components/charts/_theme';
import type { Granularity } from '@/components/domain/period-filter-bar';
import { Switch } from '@/components/ui/switch';
import type { Locale } from '@/i18n/locale';
import type { ConsumptionRow } from '@/lib/api/types';
import { formatPeriod } from '@/lib/format-period';

export type SeriesToggles = { active: boolean; inductive: boolean; capacitive: boolean };

export type SeriesChartViewProps = {
  rows: ConsumptionRow[];
  granularity: Granularity;
  show: SeriesToggles;
  onShowChange: (show: SeriesToggles) => void;
  loading?: boolean;
};

/** The consumption tab's chart with its per-series toggles (01 §7.3). */
export function SeriesChartView({ rows, granularity, show, onShowChange, loading = false }: SeriesChartViewProps) {
  const t = useTranslations('consumption');
  const locale = useLocale() as Locale;

  const series = useMemo<ChartSeries[]>(() => {
    const all: ChartSeries[] = [
      { key: 'active', label: t('columns.active'), kind: 'consumption', unit: 'kWh' },
      { key: 'inductive', label: t('columns.inductive'), kind: 'reactiveInductive', unit: 'kVArh' },
      { key: 'capacitive', label: t('columns.capacitive'), kind: 'reactiveCapacitive', unit: 'kVArh' },
    ];
    return all.filter((s) => show[s.key as keyof SeriesToggles]);
  }, [show, t]);

  const data = useMemo<Datum[]>(
    () =>
      rows.map((row) => ({
        x: formatPeriod(row.period_start, granularity, locale),
        active: row.active_import ?? null,
        inductive: row.reactive_inductive_import ?? null,
        capacitive: row.reactive_capacitive_import ?? null,
      })),
    [rows, granularity, locale],
  );

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-4">
        {(['active', 'inductive', 'capacitive'] as const).map((key) => (
          <Switch
            key={key}
            label={t(`columns.${key}`)}
            checked={show[key]}
            onCheckedChange={(on) => onShowChange({ ...show, [key]: on })}
          />
        ))}
      </div>
      <LineChart
        title={t('chart.title')}
        description={t('chart.description')}
        xLabel={t('columns.period')}
        data={data}
        series={series}
        loading={loading}
        empty={{ title: t('chart.empty'), description: t('chart.emptyHint') }}
      />
    </div>
  );
}
