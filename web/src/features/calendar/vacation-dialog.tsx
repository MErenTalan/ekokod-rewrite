'use client';

import { Trash2 } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { DatePicker } from '@/components/ui/date-picker';
import { Dialog } from '@/components/ui/dialog';
import { IconButton } from '@/components/ui/icon-button';
import { Input } from '@/components/ui/input';
import type { VacationPeriod, VacationsPutRequest } from '@/lib/api/types';

/** Monday-first display order over the API's Sunday-zero weekday numbers. */
const WEEKDAYS = [
  { day: 1, key: 'monday' },
  { day: 2, key: 'tuesday' },
  { day: 3, key: 'wednesday' },
  { day: 4, key: 'thursday' },
  { day: 5, key: 'friday' },
  { day: 6, key: 'saturday' },
  { day: 0, key: 'sunday' },
] as const;

export type VacationDialogViewProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  weekendDays: number[];
  periods: VacationPeriod[];
  onSave: (values: VacationsPutRequest) => void;
  readOnly?: boolean;
  saving?: boolean;
};

/**
 * Weekend days and vacation periods (01 §7.18). They decide the load profile's
 * weekday/weekend split and the ML day types (R137), so the dialog says when a
 * configuration would leave no working day at all.
 */
export function VacationDialogView({
  open,
  onOpenChange,
  weekendDays,
  periods,
  onSave,
  readOnly = false,
  saving = false,
}: VacationDialogViewProps) {
  const t = useTranslations('calendar');
  const weekdays = useTranslations('calendar.weekdays');
  const common = useTranslations('common');
  const [days, setDays] = useState<number[]>(weekendDays);
  const [rows, setRows] = useState<VacationPeriod[]>(periods);

  const invalid = rows.find((row) => row.end_date < row.start_date);

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('vacationsTitle')}
      size="lg"
      footer={
        readOnly ? undefined : (
          <Button
            loading={saving}
            disabled={Boolean(invalid)}
            onClick={() => onSave({ weekend_days: days, periods: rows })}
          >
            {common('save')}
          </Button>
        )
      }
    >
      <div className="flex flex-col gap-4">
        <fieldset className="flex flex-col gap-2">
          <legend className="text-foreground type-small font-semibold">{t('weeklyDays')}</legend>
          <div className="flex flex-wrap gap-3">
            {WEEKDAYS.map(({ day, key }) => (
              <Checkbox
                key={key}
                label={weekdays(key)}
                checked={days.includes(day)}
                disabled={readOnly}
                onCheckedChange={(checked) => setDays(checked ? [...days, day] : days.filter((d) => d !== day))}
              />
            ))}
          </div>
          {days.length === 7 ? <Alert tone="warning" title={t('allDaysWarning')} /> : null}
        </fieldset>

        <fieldset className="flex flex-col gap-3">
          <legend className="text-foreground type-small font-semibold">{t('periods')}</legend>
          {rows.length === 0 ? <p className="text-foreground-muted type-small">{t('noPeriods')}</p> : null}
          {rows.map((row, index) => (
            <div key={index} className="flex flex-wrap items-end gap-2">
              <DatePicker
                label={t('periodStart')}
                value={row.start_date}
                disabled={readOnly}
                onValueChange={(date) => setRows(rows.map((r, i) => (i === index ? { ...r, start_date: date ?? r.start_date } : r)))}
              />
              <DatePicker
                label={t('periodEnd')}
                value={row.end_date}
                disabled={readOnly}
                error={row.end_date < row.start_date ? t('periodInvalid') : undefined}
                onValueChange={(date) => setRows(rows.map((r, i) => (i === index ? { ...r, end_date: date ?? r.end_date } : r)))}
              />
              <Input
                label={t('periodDescription')}
                value={row.description ?? ''}
                disabled={readOnly}
                onChange={(e) => setRows(rows.map((r, i) => (i === index ? { ...r, description: e.target.value } : r)))}
              />
              {readOnly ? null : (
                <IconButton
                  label={t('removePeriod', { description: row.description || row.start_date })}
                  icon={Trash2}
                  size="sm"
                  onClick={() => setRows(rows.filter((_, i) => i !== index))}
                />
              )}
            </div>
          ))}
          {readOnly ? null : (
            <Button
              variant="secondary"
              size="sm"
              className="self-start"
              onClick={() => {
                const today = new Date().toISOString().slice(0, 10);
                setRows([...rows, { start_date: today, end_date: today, description: '' }]);
              }}
            >
              {t('addPeriod')}
            </Button>
          )}
        </fieldset>
      </div>
    </Dialog>
  );
}
