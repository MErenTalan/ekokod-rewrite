'use client';

import { useTranslations } from 'next-intl';
import { useCallback, useMemo } from 'react';

import { BarChart } from '@/components/charts/bar-chart';
import type { ChartSeries, Datum } from '@/components/charts/_theme';
import { Select } from '@/components/ui/select';
import { StatTile } from '@/components/ui/stat-tile';
import { StaggerGrid } from '@/components/ui/stagger-grid';
import { Switch } from '@/components/ui/switch';
import type { ConsumptionGrouped, GroupedPeriod } from '@/lib/api/types';
import { formatDate, formatNumber } from '@/lib/format';
import type { Locale } from '@/i18n/locale';
import { useLocale } from 'next-intl';

import type { SeriesToggles } from './series-chart';

/** The five groupings of 01 §7.3, as the API names them (R193). */
export const GROUP_BY = ['daily', 'week', 'day_type', 'season', 'season_day_type'] as const;
export type GroupBy = (typeof GROUP_BY)[number];

const GROUP_LABEL = {
  daily: 'daily',
  week: 'week',
  day_type: 'dayType',
  season: 'season',
  season_day_type: 'seasonDayType',
} as const;

const SEASONS = { winter: 'winter', spring: 'spring', summer: 'summer', autumn: 'autumn' } as const;

export type DetailedGraphsViewProps = {
  data: ConsumptionGrouped | null;
  groupBy: GroupBy;
  onGroupByChange: (groupBy: GroupBy) => void;
  compare: boolean;
  onCompareChange: (compare: boolean) => void;
  show: SeriesToggles;
  onShowChange: (show: SeriesToggles) => void;
  loading?: boolean;
};

/** The "detailed graphs" tab: grouped totals, the previous period, and statistics. */
export function DetailedGraphsView({
  data,
  groupBy,
  onGroupByChange,
  compare,
  onCompareChange,
  show,
  onShowChange,
  loading = false,
}: DetailedGraphsViewProps) {
  const t = useTranslations('consumption');
  const detailed = useTranslations('consumption.detailed');
  const locale = useLocale() as Locale;

  const label = useCallback(
    (key: string): string => {
      if (groupBy === 'day_type') return detailed(key === 'weekend' ? 'weekend' : 'weekday');
      if (groupBy === 'week') return detailed('weekLabel', { date: formatDate(key, locale) });
      if (groupBy === 'daily') return formatDate(key, locale);
      const [season, year, dayType] = key.split('-');
      const name = `${detailed(SEASONS[season as keyof typeof SEASONS] ?? 'winter')} ${year}`;
      return dayType ? `${name} · ${detailed(dayType === 'weekend' ? 'weekend' : 'weekday')}` : name;
    },
    [groupBy, detailed, locale],
  );

  const series = useMemo<ChartSeries[]>(() => {
    const all: ChartSeries[] = [
      { key: 'active', label: t('columns.active'), kind: 'consumption', unit: 'kWh' },
      { key: 'inductive', label: t('columns.inductive'), kind: 'reactiveInductive', unit: 'kVArh' },
      { key: 'capacitive', label: t('columns.capacitive'), kind: 'reactiveCapacitive', unit: 'kVArh' },
    ];
    return all.filter((s) => show[s.key as keyof SeriesToggles]);
  }, [show, t]);

  const rows = (period: GroupedPeriod | undefined): Datum[] =>
    (period?.groups ?? []).map((group) => ({
      x: label(group.key),
      active: group.active_import ?? null,
      inductive: group.reactive_inductive_import ?? null,
      capacitive: group.reactive_capacitive_import ?? null,
    }));

  const comparison = useMemo<Datum[]>(() => {
    if (!data?.previous) return [];
    return data.current.groups.map((group, i) => ({
      x: label(group.key),
      current: group.active_import ?? null,
      previous: data.previous?.groups[i]?.active_import ?? null,
    }));
  }, [data, label]);

  const stats = data?.current.statistics;
  // StatTile takes a preformatted figure, so the decimals are formatted here (D9).
  const figure = (value?: string | null) => (value == null ? '—' : formatNumber(value));

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end gap-4">
        <div className="w-56">
          <Select
            label={detailed('groupBy')}
            options={GROUP_BY.map((value) => ({ value, label: detailed(GROUP_LABEL[value]) }))}
            value={groupBy}
            onValueChange={(value) => onGroupByChange(value as GroupBy)}
          />
        </div>
        <Switch label={detailed('compare')} checked={compare} onCheckedChange={onCompareChange} />
        {(['active', 'inductive', 'capacitive'] as const).map((key) => (
          <Switch
            key={key}
            label={t(`columns.${key}`)}
            checked={show[key]}
            onCheckedChange={(on) => onShowChange({ ...show, [key]: on })}
          />
        ))}
      </div>

      <BarChart
        title={detailed('chartTitle')}
        description={detailed('chartDescription')}
        xLabel={detailed('groupBy')}
        data={rows(data?.current)}
        series={series}
        loading={loading}
        empty={{ title: detailed('empty'), description: detailed('emptyHint') }}
      />

      {compare ? (
        <BarChart
          title={detailed('comparisonTitle')}
          description={detailed('comparisonDescription')}
          xLabel={detailed('groupBy')}
          data={comparison}
          series={[
            { key: 'current', label: detailed('current'), kind: 'current', unit: 'kWh' },
            { key: 'previous', label: detailed('previous'), kind: 'previous', unit: 'kWh' },
          ]}
          loading={loading}
          empty={{ title: detailed('empty'), description: detailed('emptyHint') }}
        />
      ) : null}

      <StaggerGrid className="sm:grid-cols-2 xl:grid-cols-4">
        <StatTile label={detailed('total')} value={figure(stats?.total)} unit="kWh" />
        <StatTile label={detailed('average')} value={figure(stats?.average)} unit="kWh" />
        <StatTile label={detailed('peak')} value={figure(stats?.peak?.value)} unit="kWh" hint={stats?.peak ? label(stats.peak.key) : undefined} />
        <StatTile label={detailed('valley')} value={figure(stats?.valley?.value)} unit="kWh" hint={stats?.valley ? label(stats.valley.key) : undefined} />
      </StaggerGrid>
    </div>
  );
}
