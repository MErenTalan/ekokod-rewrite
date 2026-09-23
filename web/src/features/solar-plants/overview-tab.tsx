'use client';

import { useTranslations } from 'next-intl';

import { BarChart } from '@/components/charts/bar-chart';
import { DataQualityBadge } from '@/components/domain/data-quality-badge';
import { Card } from '@/components/ui/card';
import { EmptyState } from '@/components/ui/empty-state';
import { formatNumber } from '@/lib/format';
import type { PlantProductionSeries, PlantRealtime, PlantRevenue, RevenuePeriod } from '@/lib/api/types';

import { FigureCard } from './figure-card';
import { pointLabel, shortDate, shortDateTime } from './format';

export type OverviewTabViewProps = {
  realtime?: PlantRealtime;
  revenue?: PlantRevenue;
  monthDaily?: PlantProductionSeries;
  history?: PlantProductionSeries;
  loading?: boolean;
  /** Part B slots: weather and environmental panels. */
  aside?: React.ReactNode;
};

/** §7.7's overview: six summary figures (R282), four revenue cards (R283) and two charts. */
export function OverviewTabView({ realtime, revenue, monthDaily, history, loading = false, aside }: OverviewTabViewProps) {
  const t = useTranslations('solarPlants');
  const stale = realtime?.stale && realtime.as_of
    ? { state: 'incomplete' as const, reason: t('summary.staleReason', { time: shortDateTime(realtime.as_of) }) }
    : undefined;

  return (
    <div className="flex flex-col gap-6">
      <section aria-label={t('tabs.overview')} className="flex flex-col gap-2">
        {stale ? <p className="text-foreground-muted type-caption">{t('summary.stale')} — {stale.reason}</p> : null}
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <FigureCard label={t('summary.activePower')} value={realtime?.active_power_kw} unit="kW" quality={stale} />
          <FigureCard label={t('summary.utilisation')} value={realtime?.capacity_utilisation_pct} unit="percent" quality={stale} />
          <FigureCard label={t('summary.today')} value={realtime?.yield_today_kwh} unit="kWh" quality={stale} />
          <FigureCard label={t('summary.month')} value={realtime?.yield_month_kwh} unit="kWh" quality={stale} />
          <FigureCard label={t('summary.year')} value={realtime?.yield_year_kwh} unit="kWh" quality={stale} />
          <FigureCard label={t('summary.total')} value={realtime?.yield_total_kwh} unit="kWh" quality={stale} />
        </div>
        {realtime?.as_of ? <p className="text-foreground-muted type-caption">{t('summary.asOf', { time: shortDateTime(realtime.as_of) })}</p> : null}
      </section>

      <RevenueCards revenue={revenue} />

      <div className="grid gap-6 xl:grid-cols-2">
        <Card>
          <BarChart
            title={t('charts.monthDaily')}
            description={t('charts.monthDailyDescription')}
            xLabel={t('charts.day')}
            loading={loading}
            data={(monthDaily?.points ?? []).map((p) => ({ x: pointLabel(p.ts, 'day'), production: p.production_kwh ?? null }))}
            series={[{ key: 'production', label: t('charts.production'), kind: 'generation', unit: 'kWh' }]}
            empty={{ title: t('charts.empty'), description: t('charts.emptyDescription') }}
          />
        </Card>
        <Card>
          <BarChart
            title={t('charts.history')}
            description={t('charts.historyDescription')}
            xLabel={t('charts.month')}
            loading={loading}
            data={(history?.points ?? []).map((p) => ({ x: pointLabel(p.ts, 'month'), production: p.production_kwh ?? null }))}
            series={[{ key: 'production', label: t('charts.production'), kind: 'generation', unit: 'kWh' }]}
            empty={{ title: t('charts.empty'), description: t('charts.emptyDescription') }}
          />
        </Card>
      </div>
      {aside}
    </div>
  );
}

function RevenueCards({ revenue }: { revenue?: PlantRevenue }) {
  const t = useTranslations('solarPlants');
  if (!revenue) return null;
  if (!revenue.available) {
    return (
      <Card>
        <EmptyState title={t('revenue.unavailable')} description={t('revenue.unavailableDescription')} />
      </Card>
    );
  }
  const periods: [string, RevenuePeriod | undefined][] = [
    [t('revenue.daily'), revenue.daily], [t('revenue.monthly'), revenue.monthly],
    [t('revenue.yearly'), revenue.yearly], [t('revenue.total'), revenue.total],
  ];
  return (
    <section aria-label={t('revenue.title')} className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {periods.map(([label, p]) => (
        <div key={label} className="flex min-w-0 flex-col gap-2 rounded-lg border border-border bg-surface-raised p-4 in-data-[card=shadow]:border-card-edge in-data-[card=shadow]:shadow-md">
          <div className="flex items-start justify-between gap-2">
            <p className="text-foreground-muted type-caption">{label}</p>
            {p?.partial ? (
              <DataQualityBadge quality={{ state: 'incomplete', reason: t('revenue.partialReason', { days: p.unpriced_days }) }} />
            ) : null}
          </div>
          {p && p.amounts.length > 0 ? (
            p.amounts.map((m) => (
              <p key={m.currency} className="flex items-baseline gap-1.5">
                <span className="type-metric">{formatNumber(m.amount, { minFractionDigits: 2, maxFractionDigits: 2 })}</span>
                <span className="text-foreground-muted type-small">{m.currency}</span>
              </p>
            ))
          ) : (
            <p className="text-foreground-muted type-body-lg">{t('noData')}</p>
          )}
          {p?.since ? <p className="text-foreground-muted type-caption">{t('revenue.since', { date: shortDate(p.since) })}</p> : null}
        </div>
      ))}
    </section>
  );
}
