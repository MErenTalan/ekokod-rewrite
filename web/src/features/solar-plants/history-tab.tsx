'use client';

import { useTranslations } from 'next-intl';

import { BarChart } from '@/components/charts/bar-chart';
import { DataQualityBadge } from '@/components/domain/data-quality-badge';
import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { DateRangePicker, type DateRange } from '@/components/ui/date-range-picker';
import { RadioGroup } from '@/components/ui/radio-group';
import type { PlantProductionPoint, PlantProductionSeries, SolarTariff } from '@/lib/api/types';
import { formatNumber } from '@/lib/format';

import { FigureCard } from './figure-card';

import { istanbulDate, pointLabel, rangeTooLong } from './format';

export type Granularity = 'hour' | 'day' | 'month';

export type HistorySummary = {
  totalKwh: number;
  averageKwh: number;
  maxKwh: number;
  revenue: { currency: string; amount: number }[];
  /** Some production fell before the first price and earned nothing here. */
  unpriced: boolean;
};

/** The legacy chart's footer (total, average, maximum, revenue), over the points that have data. */
export function historySummary(points: PlantProductionPoint[], tariffs: SolarTariff[] = []): HistorySummary | null {
  const values = points.filter((p) => p.production_kwh != null).map((p) => ({ date: istanbulDate(p.ts), kwh: Number(p.production_kwh) }));
  if (values.length === 0) return null;
  const byDate = [...tariffs].sort((a, b) => b.effective_from.localeCompare(a.effective_from));
  const revenue = new Map<string, number>();
  let unpriced = false;
  for (const v of values) {
    const tariff = byDate.find((t) => t.effective_from <= v.date);
    if (!tariff) {
      unpriced ||= v.kwh > 0;
      continue;
    }
    const currency = tariff.currency ?? 'TRY';
    revenue.set(currency, (revenue.get(currency) ?? 0) + v.kwh * Number(tariff.feed_in_tariff));
  }
  const total = values.reduce((sum, v) => sum + v.kwh, 0);
  return {
    totalKwh: total,
    averageKwh: total / values.length,
    maxKwh: Math.max(...values.map((v) => v.kwh)),
    revenue: [...revenue].map(([currency, amount]) => ({ currency, amount })),
    unpriced: unpriced && tariffs.length > 0,
  };
}

export type HistoryTabViewProps = {
  granularity: Granularity;
  range: DateRange;
  series?: PlantProductionSeries;
  /** The plant's sale prices; each point is priced at the one in force that day. */
  tariffs?: SolarTariff[];
  loading?: boolean;
  onGranularityChange: (g: Granularity) => void;
  onRangeChange: (r: DateRange) => void;
  onExport: () => void;
};

/** §7.7's history tab: day/month/year selector → hour/day/month points (R284), chart, table and Excel. */
export function HistoryTabView({ granularity, range, series, tariffs, loading = false, onGranularityChange, onRangeChange, onExport }: HistoryTabViewProps) {
  const t = useTranslations('solarPlants');
  const tooLong = rangeTooLong(granularity, range.from, range.to);
  const summary = tooLong || loading ? null : historySummary(series?.points ?? [], tariffs);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end gap-4">
        <RadioGroup
          label={t('history.granularity')}
          orientation="horizontal"
          value={granularity}
          onValueChange={(v) => onGranularityChange(v as Granularity)}
          options={(['hour', 'day', 'month'] as const).map((g) => ({ value: g, label: t(`history.${g}`) }))}
        />
        <DateRangePicker label={t('history.range')} value={range} onValueChange={onRangeChange} />
        <Button variant="secondary" onClick={onExport} disabled={tooLong}>{t('history.export')}</Button>
        {series?.mixed_basis ? <DataQualityBadge quality={{ state: 'incomplete', reason: t('history.mixedReason') }} /> : null}
      </div>
      {tooLong ? (
        <Alert tone="warning" title={t(`history.tooLong.${granularity}`)} />
      ) : (
        <BarChart
          title={t('history.title')}
          description={t('history.description')}
          xLabel={t(`charts.${granularity}`)}
          loading={loading}
          data={(series?.points ?? []).map((p) => ({ x: pointLabel(p.ts, granularity), production: p.production_kwh ?? null }))}
          series={[{ key: 'production', label: t('charts.production'), kind: 'generation', unit: 'kWh' }]}
          empty={{ title: t('charts.empty'), description: t('charts.emptyDescription') }}
        />
      )}
      {summary ? (
        <section aria-label={t('history.summary')} className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <FigureCard label={t('history.total')} value={String(summary.totalKwh / 1000)} unit="MWh" />
          <FigureCard label={t('history.average')} value={String(summary.averageKwh)} unit="kWh" />
          <FigureCard label={t('history.max')} value={String(summary.maxKwh)} unit="kWh" />
          {summary.revenue.length > 0 ? (
            <div className="flex min-w-0 flex-col gap-2 rounded-lg border border-border bg-surface-raised p-4 in-data-[card=shadow]:border-card-edge in-data-[card=shadow]:shadow-md">
              <div className="flex items-start justify-between gap-2">
                <p className="text-foreground-muted type-caption">{t('history.revenue')}</p>
                {summary.unpriced ? <DataQualityBadge quality={{ state: 'incomplete', reason: t('history.unpricedReason') }} /> : null}
              </div>
              {summary.revenue.map((m) => (
                <p key={m.currency} className="flex items-baseline gap-1.5">
                  <span className="type-metric">{formatNumber(m.amount, { minFractionDigits: 2, maxFractionDigits: 2 })}</span>
                  <span className="text-foreground-muted type-small">{m.currency}</span>
                </p>
              ))}
            </div>
          ) : null}
        </section>
      ) : null}
    </div>
  );
}
