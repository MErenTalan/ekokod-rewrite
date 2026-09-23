'use client';

import { useTranslations } from 'next-intl';

import { BarChart } from '@/components/charts/bar-chart';
import { DataQualityBadge } from '@/components/domain/data-quality-badge';
import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { DateRangePicker, type DateRange } from '@/components/ui/date-range-picker';
import { RadioGroup } from '@/components/ui/radio-group';
import type { PlantProductionSeries } from '@/lib/api/types';

import { pointLabel, rangeTooLong } from './format';

export type Granularity = 'hour' | 'day' | 'month';

export type HistoryTabViewProps = {
  granularity: Granularity;
  range: DateRange;
  series?: PlantProductionSeries;
  loading?: boolean;
  onGranularityChange: (g: Granularity) => void;
  onRangeChange: (r: DateRange) => void;
  onExport: () => void;
};

/** §7.7's history tab: day/month/year selector → hour/day/month points (R284), chart, table and Excel. */
export function HistoryTabView({ granularity, range, series, loading = false, onGranularityChange, onRangeChange, onExport }: HistoryTabViewProps) {
  const t = useTranslations('solarPlants');
  const tooLong = rangeTooLong(granularity, range.from, range.to);
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
    </div>
  );
}
