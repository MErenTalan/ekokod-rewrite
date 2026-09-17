'use client';

import type { ColumnDef } from '@tanstack/react-table';
import { useLocale, useTranslations } from 'next-intl';
import { useMemo, useState, type ReactNode } from 'react';

import { BarChart } from '@/components/charts/bar-chart';
import { LineChart } from '@/components/charts/line-chart';
import type { ChartSeries, Datum } from '@/components/charts/_theme';
import { PeriodFilterBar, type Granularity } from '@/components/domain/period-filter-bar';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { DataTable } from '@/components/ui/data-table';
import { Alert } from '@/components/ui/alert';
import { Switch } from '@/components/ui/switch';
import type { DateRange } from '@/components/ui/date-range-picker';
import type { Locale } from '@/i18n/locale';
import type { ConsumptionRow } from '@/lib/api/types';
import { formatNumber } from '@/lib/format';

import { formatPeriod } from './format-period';

/** The three series the dashboard chart can draw (01 §7.2). */
export type SeriesToggles = { active: boolean; inductive: boolean; capacitive: boolean };

export const CHART_POINT_CAP = 50;

export type ConsumptionPanelViewProps = {
  rows: ConsumptionRow[];
  /** Same months a year earlier, for the year-over-year bars; null hides them. */
  previousYear: ConsumptionRow[] | null;
  granularity: Granularity;
  range: DateRange;
  today: string;
  onApply: (granularity: Granularity, range: DateRange) => void;
  show: SeriesToggles;
  onShowChange: (show: SeriesToggles) => void;
  actions?: ReactNode;
  loading?: boolean;
};

/**
 * The full-width consumption panel of 01 §7.2: period and range, per-series
 * toggles, a chart capped at the first 50 points with a notice, the paginated
 * table beneath it, and the year-over-year bars when the range allows.
 */
export function ConsumptionPanelView({
  rows,
  previousYear,
  granularity,
  range,
  today,
  onApply,
  show,
  onShowChange,
  actions,
  loading = false,
}: ConsumptionPanelViewProps) {
  const t = useTranslations('dashboard.consumption');
  const units = useTranslations('units');
  const locale = useLocale() as Locale;
  const [draft, setDraft] = useState({ granularity, range });

  const series = useMemo<ChartSeries[]>(() => {
    const all: ChartSeries[] = [
      { key: 'active', label: t('active'), kind: 'consumption', unit: 'kWh' },
      { key: 'inductive', label: t('inductive'), kind: 'reactiveInductive', unit: 'kVArh' },
      { key: 'capacitive', label: t('capacitive'), kind: 'reactiveCapacitive', unit: 'kVArh' },
    ];
    return all.filter((s) => show[s.key as keyof SeriesToggles]);
  }, [show, t]);

  const data = useMemo<Datum[]>(
    () =>
      rows.slice(0, CHART_POINT_CAP).map((row) => ({
        x: formatPeriod(row.period_start, granularity, locale),
        active: row.active_import ?? null,
        inductive: row.reactive_inductive_import ?? null,
        capacitive: row.reactive_capacitive_import ?? null,
      })),
    [rows, granularity, locale],
  );

  const yoy = useMemo<Datum[] | null>(() => {
    if (!previousYear) return null;
    return rows.map((row, i) => ({
      x: formatPeriod(row.period_start, granularity, locale),
      current: row.active_import ?? null,
      previous: previousYear[i]?.active_import ?? null,
    }));
  }, [rows, previousYear, granularity, locale]);

  const columns = useMemo<ColumnDef<ConsumptionRow, unknown>[]>(
    () => [
      {
        id: 'period',
        header: t('period'),
        accessorFn: (row) => row.period_start,
        cell: ({ row }) => formatPeriod(row.original.period_start, granularity, locale),
        enableHiding: false,
      },
      {
        id: 'active',
        header: `${t('active')} (${units('kWh')})`,
        accessorFn: (row) => Number(row.active_import ?? 0),
        meta: { numeric: true },
        cell: ({ row }) => formatNumber(row.original.active_import ?? null),
      },
      {
        id: 'inductive',
        header: `${t('inductive')} (${units('kVArh')})`,
        accessorFn: (row) => Number(row.reactive_inductive_import ?? 0),
        meta: { numeric: true },
        cell: ({ row }) => formatNumber(row.original.reactive_inductive_import ?? null),
      },
      {
        id: 'capacitive',
        header: `${t('capacitive')} (${units('kVArh')})`,
        accessorFn: (row) => Number(row.reactive_capacitive_import ?? 0),
        meta: { numeric: true },
        cell: ({ row }) => formatNumber(row.original.reactive_capacitive_import ?? null),
      },
    ],
    [t, units, granularity, locale],
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <PeriodFilterBar
            granularity={draft.granularity}
            onGranularityChange={(g) => setDraft((d) => ({ ...d, granularity: g }))}
            range={draft.range}
            onRangeChange={(r) => setDraft((d) => ({ ...d, range: r }))}
            onApply={() => onApply(draft.granularity, draft.range)}
            today={today}
            applying={loading}
          />
          {actions}
        </div>
        <div className="flex flex-wrap gap-4">
          {(['active', 'inductive', 'capacitive'] as const).map((key) => (
            <Switch key={key} label={t(key)} checked={show[key]} onCheckedChange={(on) => onShowChange({ ...show, [key]: on })} />
          ))}
        </div>
        {rows.length > CHART_POINT_CAP ? (
          <Alert tone="info" title={t('capNotice', { shown: CHART_POINT_CAP, total: rows.length })} />
        ) : null}
        <LineChart
          title={t('chartTitle')}
          description={t('chartDescription')}
          xLabel={t('period')}
          data={data}
          series={series}
          loading={loading}
          empty={{ title: t('empty'), description: t('emptyHint') }}
        />
        {yoy ? (
          <BarChart
            title={t('yoyTitle')}
            description={t('yoyDescription')}
            xLabel={t('period')}
            data={yoy}
            series={[
              { key: 'current', label: t('currentPeriod'), kind: 'current', unit: 'kWh' },
              { key: 'previous', label: t('previousYear'), kind: 'previous', unit: 'kWh' },
            ]}
            loading={loading}
            empty={{ title: t('empty'), description: t('emptyHint') }}
          />
        ) : (
          <p className="text-foreground-muted type-small">{t('yoyOnlyMonthly')}</p>
        )}
        <DataTable
          columns={columns}
          data={rows}
          caption={t('tableCaption')}
          getRowId={(row) => row.period_start}
          pageSize={10}
          loading={loading}
          empty={{ title: t('empty'), description: t('emptyHint') }}
        />
      </CardContent>
    </Card>
  );
}
