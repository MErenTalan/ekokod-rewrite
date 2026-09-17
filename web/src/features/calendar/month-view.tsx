'use client';

import { useLocale, useTranslations } from 'next-intl';
import { useMemo, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Popover } from '@/components/ui/popover';
import type { CalendarEvent, VacationPeriod } from '@/lib/api/types';
import type { Locale } from '@/i18n/locale';

import { eventDays, nonWorkingDay } from './event-draft';
import { visibleDays } from './range';

const WEEKDAY_KEYS = ['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday'] as const;
const CHIPS_PER_DAY = 3;

export type CalendarViewProps = {
  anchor: string;
  events: CalendarEvent[];
  weekendDays: number[];
  periods: VacationPeriod[];
  canEdit: boolean;
  onSelectDate: (date: string) => void;
  onSelectEvent: (event: CalendarEvent) => void;
};

/** An event chip: the colour is a border and a dot, never the only signal (R203). */
export function EventChip({ event, onSelect }: { event: CalendarEvent; onSelect: () => void }) {
  return (
    <button
      type="button"
      onClick={onSelect}
      style={{ borderInlineStartColor: event.colour ?? undefined }}
      className="flex w-full items-center gap-1 truncate rounded-sm border-s-4 bg-surface-sunken px-1 py-0.5 text-start text-foreground type-caption"
    >
      <span aria-hidden className="size-2 shrink-0 rounded-full" style={{ backgroundColor: event.colour ?? undefined }} />
      <span className="truncate">{event.title}</span>
    </button>
  );
}

/** The month grid of 01 §7.18: six Monday-first weeks. */
export function MonthView({ anchor, events, weekendDays, periods, canEdit, onSelectDate, onSelectEvent }: CalendarViewProps) {
  const t = useTranslations('calendar');
  const weekdays = useTranslations('calendar.weekdays');
  const locale = useLocale() as Locale;
  const [expanded, setExpanded] = useState<string | null>(null);

  const days = useMemo(() => visibleDays('month', anchor), [anchor]);
  const byDay = useMemo(() => {
    const map = new Map<string, CalendarEvent[]>();
    for (const event of events) {
      for (const day of eventDays(event)) map.set(day, [...(map.get(day) ?? []), event]);
    }
    return map;
  }, [events]);

  const dayNumber = new Intl.DateTimeFormat(locale === 'tr' ? 'tr-TR' : 'en-US', { day: 'numeric', timeZone: 'Europe/Istanbul' });
  const month = anchor.slice(0, 7);

  return (
    // Seven day columns need more width than a phone has, so the grid scrolls
    // inside its own box and the page never does (07 §11).
    <div className="flex flex-col gap-1 overflow-x-auto">
      <div className="grid min-w-3xl grid-cols-7 gap-1">
        {WEEKDAY_KEYS.map((key) => (
          <div key={key} className="px-1 text-foreground-muted type-caption">
            {weekdays(key)}
          </div>
        ))}
      </div>
      <div className="grid min-w-3xl grid-cols-7 gap-1">
        {days.map((day) => {
          const status = nonWorkingDay(day, weekendDays, periods);
          const dayEvents = byDay.get(day) ?? [];
          const hidden = dayEvents.slice(CHIPS_PER_DAY);
          return (
            <div
              key={day}
              data-day={day}
              data-non-working={status.weekend || status.vacation ? 'true' : undefined}
              className="flex min-h-24 flex-col gap-1 rounded-md border border-border p-1 data-[non-working=true]:bg-surface-sunken"
            >
              <div className="flex items-baseline justify-between gap-1">
                <span className={day.slice(0, 7) === month ? 'text-foreground type-caption' : 'text-foreground-subtle type-caption'}>
                  {dayNumber.format(new Date(`${day}T12:00:00Z`))}
                </span>
                {status.vacation ? (
                  <span className="text-foreground-muted type-caption">{status.vacation.description || t('vacation')}</span>
                ) : status.weekend ? (
                  <span className="text-foreground-muted type-caption">{t('weekend')}</span>
                ) : null}
              </div>
              {dayEvents.slice(0, CHIPS_PER_DAY).map((event) => (
                <EventChip key={event.id} event={event} onSelect={() => onSelectEvent(event)} />
              ))}
              {hidden.length > 0 ? (
                <Popover
                  open={expanded === day}
                  onOpenChange={(open) => setExpanded(open ? day : null)}
                  trigger={
                    <Button size="sm" variant="ghost">
                      {t('moreEvents', { count: hidden.length })}
                    </Button>
                  }
                >
                  <div className="flex flex-col gap-1">
                    {hidden.map((event) => (
                      <EventChip key={event.id} event={event} onSelect={() => onSelectEvent(event)} />
                    ))}
                  </div>
                </Popover>
              ) : null}
              {canEdit ? (
                <Button size="sm" variant="ghost" className="mt-auto self-start" onClick={() => onSelectDate(day)}>
                  {t('addEvent')}
                </Button>
              ) : null}
            </div>
          );
        })}
      </div>
    </div>
  );
}
