'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { DateRangePicker, type DateRange } from '@/components/ui/date-range-picker';
import { Input } from '@/components/ui/input';
import { RadioGroup } from '@/components/ui/radio-group';

export type ReportRequest = { report_type: 'ghg' | 'iso'; from: string; to: string; name?: string };

const DAY = 86_400_000;

/** R312's period rule, checked before the server does. */
export function reportRangeError(range: DateRange, today: string): 'tooLong' | 'future' | null {
  if (range.to > today) return 'future';
  return (Date.parse(range.to) - Date.parse(range.from)) / DAY + 1 > 366 ? 'tooLong' : null;
}

export type ReportFormProps = {
  today: string;
  initialRange: DateRange;
  saving: boolean;
  errors: Record<string, string>;
  onSubmit: (req: ReportRequest) => void;
};

/** R326: GHG or ISO 14064 for a period, optionally named. */
export function ReportForm({ today, initialRange, saving, errors, onSubmit }: ReportFormProps) {
  const t = useTranslations('carbon');
  const [type, setType] = useState<'ghg' | 'iso'>('ghg');
  const [range, setRange] = useState<DateRange>(initialRange);
  const [name, setName] = useState('');
  const problem = reportRangeError(range, today);
  return (
    <section aria-labelledby="carbon-report-form" className="flex flex-col gap-4 rounded-lg border border-border bg-surface-raised p-4">
      <h2 id="carbon-report-form" className="type-h3">
        {t('reporting.formTitle')}
      </h2>
      <RadioGroup
        label={t('reporting.type')}
        value={type}
        onValueChange={(v) => setType(v as 'ghg' | 'iso')}
        options={[
          { value: 'ghg', label: t('reporting.ghg') },
          { value: 'iso', label: t('reporting.iso') },
        ]}
        error={errors.report_type}
      />
      <DateRangePicker
        label={t('reporting.period')}
        value={range}
        onValueChange={setRange}
        max={today}
        required
        error={problem ? t(`reporting.${problem}`) : (errors.to ?? errors.from)}
      />
      <Input label={t('reporting.name')} value={name} maxLength={120} onChange={(e) => setName(e.target.value)} error={errors.name} />
      <Button
        className="self-start"
        loading={saving}
        disabled={problem !== null}
        onClick={() => onSubmit({ report_type: type, from: range.from, to: range.to, ...(name.trim() ? { name: name.trim() } : {}) })}
      >
        {t('reporting.generate')}
      </Button>
    </section>
  );
}
