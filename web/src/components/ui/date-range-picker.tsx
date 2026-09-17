'use client';

import { CalendarRange } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';
import { useState } from 'react';

import type { Locale } from '@/i18n/locale';
import { cn } from '@/lib/cn';
import { formatDate } from '@/lib/format';

import { Calendar, fromIsoDate, toIsoDate } from './_calendar';
import { controlClasses, Field, type FieldProps } from './field';
import { Popover } from './popover';

export type DateRange = { from: string; to: string };
export type DateRangePreset = { id: string; label: string; range: DateRange };
export type DateRangePickerProps = FieldProps & {
  value: DateRange | null;
  onValueChange: (value: DateRange) => void;
  presets?: DateRangePreset[];
  min?: string;
  max?: string;
};

export function DateRangePicker({ label, description, error, required, id, disabled, value, onValueChange, presets = [], min, max }: DateRangePickerProps) {
  const t = useTranslations('forms');
  const locale = useLocale() as Locale;
  const [open, setOpen] = useState(false);
  const selected = value ? { from: fromIsoDate(value.from), to: fromIsoDate(value.to) } : undefined;
  return (
    <Field label={label} description={description} error={error} required={required} id={id} disabled={disabled}>
      {({ controlId, labelId, describedBy, invalid }) => (
        <Popover
          open={open}
          onOpenChange={setOpen}
          label={label}
          trigger={
            <button
              id={controlId}
              type="button"
              aria-labelledby={`${labelId} ${controlId}-value`}
              aria-describedby={describedBy}
              disabled={disabled}
              data-invalid={invalid || undefined}
              className={cn(controlClasses, 'flex items-center gap-2 text-start data-invalid:border-danger')}
            >
              <CalendarRange aria-hidden className="size-4 shrink-0 text-foreground-muted" />
              <span id={`${controlId}-value`} className={cn('truncate', !value && 'text-foreground-subtle')}>
                {value ? `${formatDate(value.from, locale)} – ${formatDate(value.to, locale)}` : t('chooseDateRange')}
              </span>
            </button>
          }
        >
          <div className="flex flex-col gap-3 sm:flex-row">
            {presets.length > 0 ? (
              <div role="group" aria-label={t('presets')} className="flex flex-row flex-wrap gap-1 sm:flex-col">
                {presets.map((preset) => (
                  <button
                    key={preset.id}
                    type="button"
                    onClick={() => {
                      onValueChange(preset.range);
                      setOpen(false);
                    }}
                    className="rounded-md px-2 py-1.5 text-start text-foreground type-small whitespace-nowrap hover:bg-surface-sunken pointer-coarse:min-h-11"
                  >
                    {preset.label}
                  </button>
                ))}
              </div>
            ) : null}
            <Calendar
              mode="range"
              numberOfMonths={2}
              selected={selected}
              defaultMonth={selected?.from}
              disabled={[...(min ? [{ before: fromIsoDate(min) }] : []), ...(max ? [{ after: fromIsoDate(max) }] : [])]}
              onSelect={(range) => {
                if (range?.from && range.to) onValueChange({ from: toIsoDate(range.from), to: toIsoDate(range.to) });
              }}
            />
          </div>
        </Popover>
      )}
    </Field>
  );
}
