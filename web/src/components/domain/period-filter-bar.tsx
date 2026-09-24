'use client';

import { useTranslations } from 'next-intl';

import { Button } from '../ui/button';
import { DateRangePicker, type DateRange } from '../ui/date-range-picker';
import { Select } from '../ui/select';

export type Granularity = 'hourly' | 'daily' | 'monthly' | 'yearly';
export type PresetId = 'last7Days' | 'lastMonth' | 'last6Months' | 'thisYear' | 'lastYear';

const iso = (d: Date) => d.toISOString().slice(0, 10);
const utc = (y: number, m: number, d: number) => new Date(Date.UTC(y, m - 1, d));

/** Presets from an injected 'YYYY-MM-DD' today (no clock in the component); UTC date arithmetic only. */
export function periodPresets(today: string): { id: PresetId; range: DateRange }[] {
  const [y, m, d] = today.split('-').map(Number);
  return [
    { id: 'last7Days', range: { from: iso(utc(y, m, d - 6)), to: today } },
    { id: 'lastMonth', range: { from: iso(utc(y, m - 1, 1)), to: iso(utc(y, m, 0)) } },
    { id: 'last6Months', range: { from: iso(utc(y, m - 5, 1)), to: today } },
    { id: 'thisYear', range: { from: `${y}-01-01`, to: today } },
    { id: 'lastYear', range: { from: `${y - 1}-01-01`, to: `${y - 1}-12-31` } },
  ];
}

export type PeriodFilterBarProps = {
  granularity: Granularity;
  onGranularityChange: (granularity: Granularity) => void;
  range: DateRange;
  onRangeChange: (range: DateRange) => void;
  onApply: () => void;
  today: string;
  applying?: boolean;
  allowedGranularities?: Granularity[];
};

export function PeriodFilterBar({
  granularity,
  onGranularityChange,
  range,
  onRangeChange,
  onApply,
  today,
  applying = false,
  allowedGranularities = ['hourly', 'daily', 'monthly', 'yearly'],
}: PeriodFilterBarProps) {
  const t = useTranslations('domain.period');
  const forms = useTranslations('forms');
  return (
    <div className="flex flex-wrap items-end gap-3">
      <div className="w-40">
        <Select
          label={t('granularity')}
          options={allowedGranularities.map((g) => ({ value: g, label: t(g) }))}
          value={granularity}
          onValueChange={(g) => onGranularityChange(g as Granularity)}
        />
      </div>
      <div className="w-80 max-w-full">
        <DateRangePicker
          label={t('range')}
          value={range}
          onValueChange={onRangeChange}
          max={today}
          presets={periodPresets(today).map((p) => ({ id: p.id, label: forms(p.id), range: p.range }))}
        />
      </div>
      <Button onClick={onApply} loading={applying}>
        {t('apply')}
      </Button>
    </div>
  );
}
