'use client';

import { useTranslations } from 'next-intl';

import { LineChart } from '@/components/charts/line-chart';
import type { ChartSeries, Datum } from '@/components/charts/_theme';
import type { LoadProfiles, Translator } from '@/lib/api/types';
import { formatHour } from '@/lib/dates';

import { profileLabel, profileNote, type ProfileKey } from './profile-label';

/** One profile's 24 hourly means as chart data. */
export function profileData(profiles: LoadProfiles['profiles'], keys: readonly ProfileKey[]): Datum[] {
  return Array.from({ length: 24 }, (_, hour) => {
    const point: Datum = { x: formatHour(hour) };
    for (const key of keys) point[key] = profiles[key]?.[hour] ?? null;
    return point;
  });
}

export type ProfileChartProps = {
  data: LoadProfiles | null;
  keys: readonly ProfileKey[];
  /** One chart per key (Daily, Seasonal) or all keys overlaid (Compare). */
  title?: string;
  loading?: boolean;
};

/** A 24-hour load curve with the explanatory note 01 §7.4 asks for. */
export function ProfileChart({ data, keys, title, loading = false }: ProfileChartProps) {
  const t = useTranslations('loadProfile') as unknown as Translator;
  const chart = useTranslations('loadProfile.chart');
  const series: ChartSeries[] = keys.map((key) => ({
    key,
    label: profileLabel(t, key),
    kind: key.includes('weekend') ? 'previous' : 'consumption',
    unit: 'kWh',
  }));
  const days = keys.length === 1 ? data?.days?.[keys[0]] : undefined;
  return (
    <figure className="flex flex-col gap-2" data-profile-key={keys.length === 1 ? keys[0] : undefined}>
      <LineChart
        title={title ?? profileLabel(t, keys[0])}
        description={chart('description')}
        xLabel={chart('hour')}
        data={profileData(data?.profiles ?? {}, keys)}
        series={series}
        loading={loading}
        empty={{ title: chart('empty'), description: chart('emptyHint') }}
      />
      {keys.length === 1 ? (
        <figcaption className="text-foreground-muted type-small">
          {profileNote(t, keys[0])}
          {days !== undefined ? ` · ${chart('days', { count: days })}` : ''}
        </figcaption>
      ) : null}
    </figure>
  );
}
