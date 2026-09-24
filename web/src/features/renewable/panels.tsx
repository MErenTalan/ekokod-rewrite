'use client';

import { useLocale, useTranslations } from 'next-intl';
import type { ReactNode } from 'react';

import { LineChart } from '@/components/charts/line-chart';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import type { Locale } from '@/i18n/locale';
import type {
  RenewableAnalytics,
  RenewableEfficiency,
  RenewableEnvironmental,
  RenewableForecast,
  RenewableGridInteraction,
  RenewablePoint,
  RenewableRealtime,
  RenewableSystemStatus,
} from '@/lib/api/types';
import { formatDate, formatDateTime, formatNumber } from '@/lib/format';

import { FigureList, reasonKey, type Figure } from './figure-list';

type PanelProps<T> = { data?: T; loading?: boolean };

/** One panel: its title always, a skeleton while loading, a sentence when the read failed, then its content. */
export function PanelCard({
  title,
  hint,
  loading,
  failed,
  children,
}: {
  title: string;
  hint?: string;
  loading?: boolean;
  failed?: boolean;
  children: ReactNode;
}) {
  const t = useTranslations('renewable');
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {hint ? <CardDescription>{hint}</CardDescription> : null}
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {failed && !loading ? (
          <p className="text-foreground-muted type-body">{t('loadFailed')}</p>
        ) : loading ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-5 w-2/3" />
            <Skeleton className="h-5 w-1/2" />
            <Skeleton className="h-5 w-3/5" />
          </div>
        ) : (
          children
        )}
      </CardContent>
    </Card>
  );
}

const u = (v?: string | number | null) => (v === null ? undefined : v);

function Trend({
  points,
  daily,
  title,
}: {
  points: RenewablePoint[];
  daily: boolean;
  title: string;
}) {
  const t = useTranslations('renewable');
  const locale = useLocale() as Locale;
  const data = points.map((p) => ({
    x: daily ? formatDate(p.ts, locale) : formatDateTime(p.ts, locale),
    kwh: p.kwh ?? null,
  }));
  return (
    <LineChart
      title={title}
      description={t('production.chartDescription')}
      xLabel={t('production.period')}
      data={data}
      series={[{ key: 'kwh', label: t('fields.generationKwh'), kind: 'generation', unit: 'kWh' }]}
      height={200}
      empty={{ title: t('production.empty'), description: t('production.emptyHint') }}
    />
  );
}

export function RealtimePanelView({ data, loading }: PanelProps<RenewableRealtime>) {
  const t = useTranslations('renewable');
  const figures: Figure[] = data
    ? [
        { field: 'current_power_kw', value: u(data.current_power_kw), unit: 'kW' },
        { field: 'today_kwh', value: u(data.today_kwh), unit: 'kWh' },
        { field: 'max_power_kw', value: u(data.max_power_kw), unit: 'kW' },
        { field: 'avg_power_kw', value: u(data.avg_power_kw), unit: 'kW' },
        { field: 'status', value: u(data.status), format: 'text' },
        { field: 'system_efficiency_pct', value: u(data.system_efficiency_pct), unit: 'percent' },
      ]
    : [];
  return (
    <PanelCard
      title={t('panels.realtime')}
      hint={t('panels.realtimeHint')}
      loading={loading}
      failed={!data}
    >
      {data ? (
        <>
          <FigureList figures={figures} unavailable={data.unavailable} />
          <Trend points={data.series_24h} daily={false} title={t('panels.series24h')} />
        </>
      ) : null}
    </PanelCard>
  );
}

export function GridPanelView({ data, loading }: PanelProps<RenewableGridInteraction>) {
  const t = useTranslations('renewable');
  return (
    <PanelCard title={t('panels.grid')} loading={loading} failed={!data}>
      {data ? (
        <FigureList
          unavailable={data.unavailable}
          figures={[
            { field: 'direction', value: u(data.direction), format: 'text' },
            { field: 'today_import_kwh', value: u(data.today_import_kwh), unit: 'kWh' },
            { field: 'today_export_kwh', value: u(data.today_export_kwh), unit: 'kWh' },
            { field: 'power_factor', value: u(data.power_factor), digits: 3 },
            { field: 'voltage_v', value: u(data.voltage_v), unit: 'V' },
            { field: 'frequency_hz', value: u(data.frequency_hz), unit: 'Hz' },
            {
              field: 'import_price',
              value: u(data.import_price),
              format: 'money',
              currency: data.currency,
              digits: 4,
            },
            {
              field: 'export_price',
              value: u(data.export_price),
              format: 'money',
              currency: data.currency,
              digits: 4,
            },
            {
              field: 'net_today',
              value: u(data.net_today),
              format: 'money',
              currency: data.currency,
            },
          ]}
        />
      ) : null}
    </PanelCard>
  );
}

export function EnvironmentalPanelView({ data, loading }: PanelProps<RenewableEnvironmental>) {
  const t = useTranslations('renewable');
  return (
    <PanelCard title={t('panels.environmental')} loading={loading} failed={!data}>
      {data ? (
        <>
          <FigureList
            unavailable={data.unavailable}
            figures={[
              { field: 'generation_kwh', value: u(data.generation_kwh), unit: 'kWh' },
              { field: 'co2_avoided_kg', value: u(data.co2_avoided_kg), unit: 'kg' },
              { field: 'trees', value: u(data.trees), unit: 'trees' },
              { field: 'coal_kg', value: u(data.coal_kg), unit: 'kg' },
              { field: 'car_km', value: u(data.car_km), unit: 'km', digits: 1 },
              { field: 'homes', value: u(data.homes), unit: 'homes', digits: 4 },
            ]}
          />
          <ul className="flex flex-col gap-1 text-foreground-muted type-small">
            {data.grid_factor ? (
              <li>
                {`${t('fields.gridFactor')}: ${formatNumber(data.grid_factor)} ${data.grid_factor_unit ?? ''} · `}
                {t('factorSource', { source: data.grid_factor_source ?? '' })}
              </li>
            ) : null}
            {data.factors.map((f) => (
              <li key={f.key}>
                {`${t('factorLine', { caption: t(`equivalences.${reasonKey(f.key)}` as 'equivalences.equivTreeCo2KgPerYear'), factor: formatNumber(f.factor), unit: f.unit })} · `}
                {f.year
                  ? t('factorSourceYear', { source: f.source, year: String(f.year) })
                  : t('factorSource', { source: f.source })}
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </PanelCard>
  );
}

export function EfficiencyPanelView({ data, loading }: PanelProps<RenewableEfficiency>) {
  const t = useTranslations('renewable');
  return (
    <PanelCard title={t('panels.efficiency')} loading={loading} failed={!data}>
      {data ? (
        <FigureList
          unavailable={data.unavailable}
          figures={[
            { field: 'overall_pct', value: u(data.overall_pct), unit: 'percent' },
            { field: 'panel_pct', value: u(data.panel_pct), unit: 'percent' },
            { field: 'inverter_pct', value: u(data.inverter_pct), unit: 'percent' },
            { field: 'battery_pct', value: u(data.battery_pct), unit: 'percent' },
            { field: 'grid_pct', value: u(data.grid_pct), unit: 'percent' },
            {
              field: 'recommendations',
              value: data.recommendations.length ? data.recommendations.join(' · ') : undefined,
              format: 'plain',
            },
          ]}
        />
      ) : null}
    </PanelCard>
  );
}

export function ForecastPanelView({ data, loading }: PanelProps<RenewableForecast>) {
  const t = useTranslations('renewable');
  return (
    <PanelCard title={t('panels.forecast')} loading={loading} failed={!data}>
      {data ? (
        <FigureList
          unavailable={data.unavailable}
          figures={[
            {
              field: 'consumption_next_24h_kwh',
              value: u(data.consumption_next_24h_kwh),
              unit: 'kWh',
            },
            {
              field: 'consumption_next_7d_kwh',
              value: u(data.consumption_next_7d_kwh),
              unit: 'kWh',
            },
            {
              field: 'consumption_next_28d_kwh',
              value: u(data.consumption_next_28d_kwh),
              unit: 'kWh',
            },
            {
              field: 'accuracy_daily_pct',
              value: u(data.accuracy_daily_pct),
              unit: 'percent',
              digits: 1,
            },
            {
              field: 'accuracy_weekly_pct',
              value: u(data.accuracy_weekly_pct),
              unit: 'percent',
              digits: 1,
            },
            {
              field: 'accuracy_overall_pct',
              value: u(data.accuracy_overall_pct),
              unit: 'percent',
              digits: 1,
            },
            {
              field: 'estimated_generation_kwh',
              value: u(data.estimated_generation_kwh),
              unit: 'kWh',
            },
            { field: 'net_excess_kwh', value: u(data.net_excess_kwh), unit: 'kWh' },
            { field: 'weather_impact', value: u(data.weather_impact), format: 'text' },
          ]}
        />
      ) : null}
    </PanelCard>
  );
}

export function AnalyticsPanelView({ data, loading }: PanelProps<RenewableAnalytics>) {
  const t = useTranslations('renewable');
  const f = data?.financial;
  return (
    <PanelCard title={t('panels.analytics')} loading={loading} failed={!data}>
      {data && f ? (
        <>
          <FigureList
            unavailable={data.unavailable}
            figures={[
              { field: 'peak_generation_kwh', value: u(data.peak_generation_kwh), unit: 'kWh' },
              {
                field: 'average_generation_kwh',
                value: u(data.average_generation_kwh),
                unit: 'kWh',
              },
              { field: 'peak_hour', value: u(data.peak_hour), format: 'hour' },
              {
                field: 'data_availability_pct',
                value: u(data.data_availability_pct),
                unit: 'percent',
                digits: 1,
              },
              {
                field: 'system_efficiency_pct',
                value: u(data.system_efficiency_pct),
                unit: 'percent',
              },
              {
                field: 'efficiency_change_30d_pct',
                value: u(data.efficiency_change_30d_pct),
                unit: 'percent',
              },
              {
                field: 'consumption_optimisation',
                value: u(data.consumption_optimisation),
                format: 'text',
              },
              {
                field: 'maintenance_required',
                value: u(data.maintenance_required),
                format: 'text',
              },
            ]}
          />
          <Trend points={data.trend} daily title={t('panels.trend')} />
          <section className="flex flex-col gap-3" aria-labelledby="renewable-financial">
            <h4 id="renewable-financial" className="text-foreground type-h4">
              {t('panels.financial')}
            </h4>
            <FigureList
              unavailable={f.unavailable}
              figures={[
                {
                  field: 'import_price',
                  value: u(f.import_price),
                  format: 'money',
                  currency: f.currency,
                  digits: 4,
                },
                {
                  field: 'export_price',
                  value: u(f.export_price),
                  format: 'money',
                  currency: f.currency,
                  digits: 4,
                },
                {
                  field: 'today_import_cost',
                  value: u(f.today_import_cost),
                  format: 'money',
                  currency: f.currency,
                },
                {
                  field: 'today_export_revenue',
                  value: u(f.today_export_revenue),
                  format: 'money',
                  currency: f.currency,
                },
                {
                  field: 'net_today',
                  value: u(f.net_today),
                  format: 'money',
                  currency: f.currency,
                },
                {
                  field: 'month_earnings',
                  value: u(f.month_earnings),
                  format: 'money',
                  currency: f.currency,
                },
                {
                  field: 'year_earnings',
                  value: u(f.year_earnings),
                  format: 'money',
                  currency: f.currency,
                },
                {
                  field: 'total_savings',
                  value: u(f.total_savings),
                  format: 'money',
                  currency: f.currency,
                },
                { field: 'roi_pct', value: u(f.roi_pct), unit: 'percent' },
                { field: 'payback_years', value: u(f.payback_years), unit: 'years' },
                {
                  field: 'bill_savings',
                  value: u(f.bill_savings),
                  format: 'money',
                  currency: f.currency,
                },
              ]}
            />
          </section>
        </>
      ) : null}
    </PanelCard>
  );
}

export function SystemStatusPanelView({ data, loading }: PanelProps<RenewableSystemStatus>) {
  const t = useTranslations('renewable');
  return (
    <PanelCard title={t('panels.systemStatus')} loading={loading} failed={!data}>
      {data ? (
        <FigureList
          unavailable={data.unavailable}
          figures={[
            { field: 'overall', value: u(data.overall), format: 'text' },
            { field: 'monitoring', value: u(data.monitoring), format: 'text' },
            { field: 'grid_connection', value: u(data.grid_connection), format: 'text' },
            { field: 'last_reading_at', value: u(data.last_reading_at), format: 'datetime' },
            { field: 'solar_panels', value: u(data.solar_panels), format: 'text' },
            { field: 'inverter', value: u(data.inverter), format: 'text' },
            { field: 'battery', value: u(data.battery), format: 'text' },
            { field: 'security', value: u(data.security), format: 'text' },
            { field: 'total_generation_kwh', value: u(data.total_generation_kwh), unit: 'kWh' },
            {
              field: 'average_efficiency_pct',
              value: u(data.average_efficiency_pct),
              unit: 'percent',
            },
          ]}
        />
      ) : null}
    </PanelCard>
  );
}
