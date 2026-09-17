'use client';

import { useLocale, useTranslations } from 'next-intl';
import { useMemo } from 'react';

import { EmptyState } from '@/components/ui/empty-state';
import { formatDate } from '@/lib/format';
import type { Locale } from '@/i18n/locale';

import { eventDays } from './event-draft';
import type { CalendarViewProps } from './month-view';
import { EventChip } from './month-view';
import { visibleDays } from './range';

/** The agenda of 01 §7.18: the coming days that actually have something on them. */
export function AgendaView({ anchor, events, onSelectEvent }: CalendarViewProps) {
  const t = useTranslations('calendar');
  const locale = useLocale() as Locale;
  const days = useMemo(() => visibleDays('agenda', anchor), [anchor]);

  const rows = useMemo(
    () =>
      days
        .map((day) => ({
          day,
          events: events
            .filter((event) => eventDays(event).includes(day))
            .sort((a, b) => a.starts_at.localeCompare(b.starts_at)),
        }))
        .filter((row) => row.events.length > 0),
    [days, events],
  );

  if (rows.length === 0) return <EmptyState title={t('noEvents')} description={t('noEventsHint')} />;

  return (
    <ul className="flex flex-col gap-4">
      {rows.map((row) => (
        <li key={row.day} className="flex flex-col gap-2">
          <h3 className="text-foreground type-h3">{formatDate(row.day, locale)}</h3>
          <ul className="flex flex-col gap-1">
            {row.events.map((event) => (
              <li key={event.id} className="flex items-center gap-2">
                <span className="w-24 shrink-0 text-foreground-muted type-caption">
                  {event.all_day ? t('allDay') : `${event.starts_at.slice(11, 16)}–${event.ends_at.slice(11, 16)}`}
                </span>
                <EventChip event={event} onSelect={() => onSelectEvent(event)} />
              </li>
            ))}
          </ul>
        </li>
      ))}
    </ul>
  );
}
