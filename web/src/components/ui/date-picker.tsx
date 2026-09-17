'use client';

import { CalendarDays } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';
import { useState } from 'react';

import type { Locale } from '@/i18n/locale';
import { cn } from '@/lib/cn';
import { formatDate } from '@/lib/format';

import { Calendar, fromIsoDate, toIsoDate } from './_calendar';
import { controlClasses, Field, type FieldProps } from './field';
import { Popover } from './popover';

export type DatePickerProps = FieldProps & {
  value: string | null;
  onValueChange: (value: string | null) => void;
  min?: string;
  max?: string;
};

export function DatePicker({ label, description, error, required, id, disabled, value, onValueChange, min, max }: DatePickerProps) {
  const t = useTranslations('forms');
  const locale = useLocale() as Locale;
  const [open, setOpen] = useState(false);
  const selected = value ? fromIsoDate(value) : undefined;
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
              <CalendarDays aria-hidden className="size-4 shrink-0 text-foreground-muted" />
              <span id={`${controlId}-value`} className={cn('truncate', !value && 'text-foreground-subtle')}>
                {value ? formatDate(value, locale) : t('chooseDate')}
              </span>
            </button>
          }
        >
          <Calendar
            mode="single"
            selected={selected}
            defaultMonth={selected ?? (max ? fromIsoDate(max) : undefined)}
            disabled={[...(min ? [{ before: fromIsoDate(min) }] : []), ...(max ? [{ after: fromIsoDate(max) }] : [])]}
            onSelect={(date) => {
              onValueChange(date ? toIsoDate(date) : null);
              setOpen(false);
            }}
          />
        </Popover>
      )}
    </Field>
  );
}
