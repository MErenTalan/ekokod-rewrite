'use client';

import { useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import { Card } from '@/components/ui/card';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { formatNumber } from '@/lib/format';
import type { Weather } from '@/lib/api/types';

type Condition = 'clear' | 'partlyCloudy' | 'fog' | 'rain' | 'snow' | 'showers' | 'storm' | 'unknown';

/** WMO weather codes in legacy's bands (WeatherWidget.tsx). */
export function conditionOf(code?: number | null): Condition {
  if (code === undefined || code === null) return 'unknown';
  if (code === 0) return 'clear';
  if (code <= 3) return 'partlyCloudy';
  if (code <= 48) return 'fog';
  if (code <= 67) return 'rain';
  if (code <= 77) return 'snow';
  if (code <= 82) return 'showers';
  return 'storm';
}

const REASONS = {
  weather_not_configured: 'weatherNotConfigured',
  location_not_configured: 'locationNotConfigured',
  weather_unavailable: 'weatherUnavailable',
} as const;
const POTENTIAL_TONE = { high: 'success', medium: 'info', low: 'neutral' } as const;

const dayLabel = (iso: string) =>
  new Intl.DateTimeFormat('tr-TR', { weekday: 'short', day: '2-digit', month: '2-digit', timeZone: 'Europe/Istanbul' }).format(new Date(`${iso}T12:00:00Z`));

/**
 * 01 §7.8's weather panel (R291): the record's own coordinates only — with none,
 * it says so and never falls back to a city (10 item 32).
 */
export function WeatherPanelView({ weather, loading = false }: { weather?: Weather; loading?: boolean }) {
  const t = useTranslations('weather');
  const num = (v?: string | null, unit = '') => (v ? `${formatNumber(v, { maxFractionDigits: 2 })}${unit}` : t('noData'));

  return (
    <Card className="p-4 flex flex-col gap-4">
      <div>
        <h3 className="type-h3">{t('title')}</h3>
        <p className="text-foreground-muted type-caption">{t('description')}</p>
      </div>
      {loading || !weather ? (
        <Skeleton className="h-40 w-full" />
      ) : !weather.available ? (
        <EmptyState title={t(`reasons.${REASONS[weather.reason as keyof typeof REASONS] ?? 'weatherUnavailable'}`)}
          description={t(`reasons.${REASONS[weather.reason as keyof typeof REASONS] ?? 'weatherUnavailable'}Description`)} />
      ) : (
        <>
          <div className="flex flex-wrap items-baseline gap-3">
            <span className="type-metric">{weather.current?.temperature_c ? formatNumber(weather.current.temperature_c, { maxFractionDigits: 1 }) : t('noData')}</span>
            <span className="text-foreground-muted type-small">°C</span>
            <span className="type-body">{t(`conditions.${conditionOf(weather.current?.weather_code)}`)}</span>
          </div>
          <dl className="grid grid-cols-2 gap-x-6 gap-y-2 type-small sm:grid-cols-3">
            {([
              ['humidity', num(weather.current?.humidity_pct, ' %')],
              ['wind', num(weather.current?.wind_kmh, ' km/sa')],
              ['pressure', num(weather.current?.pressure_hpa, ' hPa')],
              ['visibility', num(weather.current?.visibility_km, ' km')],
              ['uv', num(weather.current?.uv_index)],
              ['precipitation', num(weather.current?.precipitation_pct, ' %')],
            ] as const).map(([key, value]) => (
              <div key={key} className="flex flex-col">
                <dt className="text-foreground-muted">{t(key)}</dt>
                <dd className="type-data">{value}</dd>
              </div>
            ))}
          </dl>
          <div className="flex flex-col gap-2">
            <h4 className="type-small font-semibold">{t('forecast')}</h4>
            <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
              {weather.days.map((d) => (
                <li key={d.date} className="flex flex-col gap-1 rounded-md border border-border p-3">
                  <span className="type-small font-semibold">{dayLabel(d.date)}</span>
                  <span className="type-small">{t(`conditions.${conditionOf(d.weather_code)}`)}</span>
                  <span className="type-data type-small">{num(d.min_c, '°')} / {num(d.max_c, '°')} · {num(d.precipitation_pct, ' %')}</span>
                  {d.potential ? (
                    <Badge tone={POTENTIAL_TONE[d.potential]}>{t('potential')}: {t(`potentials.${d.potential}`)}</Badge>
                  ) : null}
                </li>
              ))}
            </ul>
            <p className="text-foreground-muted type-caption">{t('potentialBasis')}</p>
          </div>
        </>
      )}
    </Card>
  );
}
