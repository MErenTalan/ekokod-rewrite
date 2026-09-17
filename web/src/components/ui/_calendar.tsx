'use client';

import { enUS, tr } from 'date-fns/locale';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';
import { DayPicker, type DayPickerProps } from 'react-day-picker';

/** 'YYYY-MM-DD' ↔ local calendar Date; props and callbacks never carry a Date (plan D19). */
export function fromIsoDate(iso: string): Date {
  const [y, m, d] = iso.split('-').map(Number);
  return new Date(y, m - 1, d ?? 1);
}
export function toIsoDate(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

const day = 'inline-flex size-9 items-center justify-center rounded-md type-small pointer-coarse:size-11';

/** react-day-picker themed from tokens, Monday-first, date-fns locale from next-intl. */
export function Calendar({ classNames, ...props }: DayPickerProps) {
  const locale = useLocale();
  const t = useTranslations('forms');
  return (
    <DayPicker
      {...props}
      locale={locale === 'tr' ? tr : enUS}
      weekStartsOn={1}
      showOutsideDays
      labels={{ labelNav: () => t('calendarNavigation'), labelNext: () => t('nextMonth'), labelPrevious: () => t('previousMonth') }}
      components={{
        Chevron: ({ orientation }) =>
          orientation === 'left' ? <ChevronLeft aria-hidden className="size-4" /> : <ChevronRight aria-hidden className="size-4" />,
      }}
      classNames={{
        root: 'relative',
        months: 'flex flex-col gap-4 sm:flex-row',
        month: 'flex flex-col gap-2',
        month_caption: 'flex h-9 items-center justify-center pointer-coarse:h-11',
        caption_label: 'text-foreground type-small font-semibold',
        nav: 'absolute inset-x-0 top-0 flex justify-between',
        button_previous: `${day} text-foreground hover:bg-surface-sunken disabled:opacity-40`,
        button_next: `${day} text-foreground hover:bg-surface-sunken disabled:opacity-40`,
        month_grid: 'border-collapse',
        weekdays: '',
        weekday: 'size-9 text-foreground-muted type-caption pointer-coarse:size-11',
        week: '',
        day: 'p-0 text-center',
        day_button: `${day} text-foreground transition-colors duration-(--duration-hover) hover:bg-surface-sunken disabled:cursor-not-allowed disabled:opacity-40`,
        today: '[&>button]:font-bold [&>button]:underline [&>button]:underline-offset-4',
        outside: '[&>button]:text-foreground-subtle',
        selected: '[&>button]:bg-primary [&>button]:text-on-primary [&>button]:hover:bg-primary-hover',
        range_middle: '[&>button]:rounded-none [&>button]:bg-primary-subtle [&>button]:text-foreground [&>button]:hover:bg-primary-subtle',
        hidden: 'invisible',
        ...classNames,
      }}
    />
  );
}
