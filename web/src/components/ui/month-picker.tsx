'use client';

import { CalendarDays, ChevronLeft, ChevronRight } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';
import { useState } from 'react';

import type { Locale } from '@/i18n/locale';
import { cn } from '@/lib/cn';
import { formatMonth } from '@/lib/format';

import { controlClasses, Field, type FieldProps } from './field';
import { IconButton } from './icon-button';
import { Popover } from './popover';

export type MonthPickerProps = FieldProps & {
  value: string | null;
  onValueChange: (value: string | null) => void;
  min?: string;
  max?: string;
};

const key = (year: number, month: number) => `${year}-${String(month).padStart(2, '0')}`;

export function MonthPicker({ label, description, error, required, id, disabled, value, onValueChange, min, max }: MonthPickerProps) {
  const t = useTranslations('forms');
  const locale = useLocale() as Locale;
  const [open, setOpen] = useState(false);
  const [year, setYear] = useState(() => Number((value ?? max ?? key(new Date().getFullYear(), 1)).slice(0, 4)));
  const short = new Intl.DateTimeFormat(locale === 'tr' ? 'tr-TR' : 'en-US', { month: 'short', timeZone: 'UTC' });
  return (
    <Field label={label} description={description} error={error} required={required} id={id} disabled={disabled}>
      {({ controlId, labelId, describedBy, invalid }) => (
        <Popover
          open={open}
          onOpenChange={(next) => {
            setOpen(next);
            if (next && value) setYear(Number(value.slice(0, 4)));
          }}
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
              <CalendarDays aria-hidden className="size-4 shrink-0 text-foreground-muted" />
              <span id={`${controlId}-value`} className={cn('truncate', !value && 'text-foreground-subtle')}>
                {value ? formatMonth(value, locale) : t('chooseMonth')}
              </span>
            </button>
          }
        >
          <div className="flex w-64 flex-col gap-2">
            <div className="flex items-center justify-between">
              <IconButton label={t('previousYear')} icon={ChevronLeft} size="sm" onClick={() => setYear((y) => y - 1)} />
              <span className="text-foreground type-small font-semibold" aria-live="polite">
                {year}
              </span>
              <IconButton label={t('nextYear')} icon={ChevronRight} size="sm" onClick={() => setYear((y) => y + 1)} />
            </div>
            <div className="grid grid-cols-3 gap-1">
              {Array.from({ length: 12 }, (_, i) => {
                const month = key(year, i + 1);
                const outOfRange = (min !== undefined && month < min) || (max !== undefined && month > max);
                const isSelected = month === value;
                return (
                  <button
                    key={month}
                    type="button"
                    aria-label={formatMonth(month, locale)}
                    aria-pressed={isSelected}
                    aria-disabled={outOfRange || undefined}
                    onClick={() => {
                      if (outOfRange) return;
                      onValueChange(month);
                      setOpen(false);
                    }}
                    className={cn(
                      'h-9 rounded-md text-foreground type-small capitalize transition-colors duration-(--duration-hover) hover:bg-surface-sunken pointer-coarse:min-h-11',
                      isSelected && 'bg-primary text-on-primary hover:bg-primary-hover',
                      outOfRange && 'opacity-40 hover:bg-transparent',
                    )}
                  >
                    {short.format(Date.UTC(year, i, 15))}
                  </button>
                );
              })}
            </div>
          </div>
        </Popover>
      )}
    </Field>
  );
}
