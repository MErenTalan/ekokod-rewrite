'use client';

import { useLocale, useTranslations } from 'next-intl';
import { useMemo } from 'react';

import { Button } from '@/components/ui/button';
import type { CalendarEvent } from '@/lib/api/types';
import { formatHour } from '@/lib/dates';
import { formatDate } from '@/lib/format';
import type { Locale } from '@/i18n/locale';

import { eventDays, minutesOf, nonWorkingDay } from './event-draft';
import { layoutLanes } from './layout-lanes';
import type { CalendarViewProps } from './month-view';
import { EventChip } from './month-view';
import { visibleDays } from './range';

const HOUR_HEIGHT = 48;
const HOURS = Array.from({ length: 24 }, (_, hour) => hour);

/** The week and day grids of 01 §7.18: an all-day row over a 24-hour column. */
export function TimeGridView({
  anchor,
  view,
  events,
  weekendDays,
  periods,
  canEdit,
  onSelectDate,
  onSelectEvent,
}: CalendarViewProps & { view: 'week' | 'day' }) {
  const t = useTranslations('calendar');
  const locale = useLocale() as Locale;
  const days = useMemo(() => visibleDays(view, anchor), [view, anchor]);

  const byDay = useMemo(() => {
    const map = new Map<string, { allDay: CalendarEvent[]; timed: CalendarEvent[] }>();
    for (const day of days) map.set(day, { allDay: [], timed: [] });
    for (const event of events) {
      for (const day of eventDays(event)) {
        const slot = map.get(day);
        if (!slot) continue;
        (event.all_day ? slot.allDay : slot.timed).push(event);
      }
    }
    return map;
  }, [days, events]);

  return (
    <div className="flex flex-col gap-2 overflow-x-auto">
      <div className="grid gap-1" style={{ gridTemplateColumns: `4rem repeat(${days.length}, minmax(8rem, 1fr))` }}>
        <div className="text-foreground-muted type-caption">{t('allDayRow')}</div>
        {days.map((day) => {
          const status = nonWorkingDay(day, weekendDays, periods);
          return (
            <div key={day} className="flex flex-col gap-1">
              <div className="flex items-baseline justify-between gap-1">
                <span className="text-foreground type-caption">{formatDate(day, locale)}</span>
                {status.vacation ? (
                  <span className="text-foreground-muted type-caption">{status.vacation.description || t('vacation')}</span>
                ) : status.weekend ? (
                  <span className="text-foreground-muted type-caption">{t('weekend')}</span>
                ) : null}
              </div>
              {(byDay.get(day)?.allDay ?? []).map((event) => (
                <EventChip key={event.id} event={event} onSelect={() => onSelectEvent(event)} />
              ))}
              {canEdit ? (
                <Button size="sm" variant="ghost" className="self-start" onClick={() => onSelectDate(day)}>
                  {t('addEvent')}
                </Button>
              ) : null}
            </div>
          );
        })}
      </div>

      <div className="grid gap-1" style={{ gridTemplateColumns: `4rem repeat(${days.length}, minmax(8rem, 1fr))` }}>
        <div>
          {HOURS.map((hour) => (
            <div key={hour} className="text-foreground-muted type-caption" style={{ height: HOUR_HEIGHT }}>
              {formatHour(hour)}
            </div>
          ))}
        </div>
        {days.map((day) => {
          const timed = byDay.get(day)?.timed ?? [];
          const lanes = layoutLanes(
            timed.map((event) => ({ id: event.id, startMin: minutesOf(event.starts_at), endMin: minutesOf(event.ends_at) })),
          );
          const status = nonWorkingDay(day, weekendDays, periods);
          return (
            <div
              key={day}
              data-day={day}
              data-non-working={status.weekend || status.vacation ? 'true' : undefined}
              className="relative rounded-md border border-border data-[non-working=true]:bg-surface-sunken"
              style={{ height: HOUR_HEIGHT * 24 }}
            >
              {HOURS.map((hour) => (
                <div key={hour} className="border-b border-border/60" style={{ height: HOUR_HEIGHT }} />
              ))}
              {timed.map((event) => {
                const start = minutesOf(event.starts_at);
                const end = Math.max(minutesOf(event.ends_at), start + 30);
                const lane = lanes[event.id] ?? { lane: 0, lanes: 1 };
                return (
                  <button
                    key={event.id}
                    type="button"
                    onClick={() => onSelectEvent(event)}
                    style={{
                      position: 'absolute',
                      top: (start / 60) * HOUR_HEIGHT,
                      height: ((end - start) / 60) * HOUR_HEIGHT,
                      insetInlineStart: `${(lane.lane / lane.lanes) * 100}%`,
                      width: `${100 / lane.lanes}%`,
                      borderInlineStartColor: event.colour ?? undefined,
                    }}
                    className="overflow-hidden rounded-sm border-s-4 bg-surface-raised px-1 text-start text-foreground type-caption"
                  >
                    <span className="block truncate">{event.title}</span>
                    <span className="block truncate text-foreground-muted">
                      {event.starts_at.slice(11, 16)}–{event.ends_at.slice(11, 16)}
                    </span>
                  </button>
                );
              })}
            </div>
          );
        })}
      </div>
    </div>
  );
}
