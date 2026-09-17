import type { CalendarEvent, CalendarEventFields } from '@/lib/api/types';
import { addDays } from '@/lib/dates';

export type EventDraft = {
  id?: string;
  title: string;
  allDay: boolean;
  startDate: string;
  startTime: string;
  endDate: string;
  endTime: string;
  colour: string;
};

const ISTANBUL_OFFSET = '+03:00';

export const emptyEvent = (date: string, colour: string): EventDraft => ({
  title: '',
  allDay: true,
  startDate: date,
  startTime: '09:00',
  endDate: date,
  endTime: '10:00',
  colour,
});

/** Reads an API event back into the form, in Istanbul time. */
export function eventDraft(event: CalendarEvent, fallbackColour: string): EventDraft {
  const [startDate, startTime] = event.starts_at.slice(0, 16).split('T');
  const [endDateRaw, endTime] = event.ends_at.slice(0, 16).split('T');
  return {
    id: event.id,
    title: event.title,
    allDay: event.all_day,
    startDate,
    startTime,
    // An all-day event ends at the next midnight; the form shows its last day.
    endDate: event.all_day ? addDays(endDateRaw, -1) : endDateRaw,
    endTime,
    colour: event.colour ?? fallbackColour,
  };
}

/** True when the form describes a range the API will accept (`ends_at > starts_at`). */
export function eventRangeValid(draft: EventDraft): boolean {
  if (draft.allDay) return draft.endDate >= draft.startDate;
  return `${draft.endDate}T${draft.endTime}` > `${draft.startDate}T${draft.startTime}`;
}

/**
 * The request body (R202): an all-day event covers whole Istanbul days, so it
 * ends at the midnight after its last day.
 */
export function toEventRequest(draft: EventDraft): CalendarEventFields {
  const starts = draft.allDay ? `${draft.startDate}T00:00:00${ISTANBUL_OFFSET}` : `${draft.startDate}T${draft.startTime}:00${ISTANBUL_OFFSET}`;
  const ends = draft.allDay
    ? `${addDays(draft.endDate, 1)}T00:00:00${ISTANBUL_OFFSET}`
    : `${draft.endDate}T${draft.endTime}:00${ISTANBUL_OFFSET}`;
  return { title: draft.title, all_day: draft.allDay, starts_at: starts, ends_at: ends, colour: draft.colour };
}

/** The ISO dates an event covers, for the month grid and the agenda. */
export function eventDays(event: CalendarEvent): string[] {
  const start = event.starts_at.slice(0, 10);
  const endExclusive = event.all_day ? event.ends_at.slice(0, 10) : addDays(event.ends_at.slice(0, 10), 1);
  const days: string[] = [];
  for (let day = start; day < endExclusive; day = addDays(day, 1)) days.push(day);
  return days.length > 0 ? days : [start];
}

/** Minutes from midnight, for the week and day grids. */
export const minutesOf = (iso: string): number => Number(iso.slice(11, 13)) * 60 + Number(iso.slice(14, 16));

/** Whether a date is a non-working day for the company (R137 config). */
export function nonWorkingDay(
  date: string,
  weekendDays: number[],
  periods: { start_date: string; end_date: string; description?: string | null }[],
): { weekend: boolean; vacation?: { description?: string | null } } {
  const weekday = new Date(`${date}T00:00:00Z`).getUTCDay();
  const vacation = periods.find((period) => date >= period.start_date && date <= period.end_date);
  return { weekend: weekendDays.includes(weekday), vacation };
}
