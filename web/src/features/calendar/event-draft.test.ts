import { describe, expect, it } from 'vitest';

import type { CalendarEvent } from '@/lib/api/types';
import { DEFAULT_EVENT_COLOUR, EVENT_COLOURS } from '@/styles/event-palette';

import { emptyEvent, eventDays, eventDraft, eventRangeValid, minutesOf, nonWorkingDay, toEventRequest } from './event-draft';

describe('toEventRequest', () => {
  it('covers whole Istanbul days for an all-day event (R202)', () => {
    const request = toEventRequest({ ...emptyEvent('2026-03-14', DEFAULT_EVENT_COLOUR), allDay: true, endDate: '2026-03-15' });
    expect(request.starts_at).toBe('2026-03-14T00:00:00+03:00');
    expect(request.ends_at).toBe('2026-03-16T00:00:00+03:00');
    expect(request.all_day).toBe(true);
  });

  it('keeps the typed times for a timed event', () => {
    const request = toEventRequest({
      ...emptyEvent('2026-03-14', DEFAULT_EVENT_COLOUR),
      allDay: false,
      startTime: '09:30',
      endDate: '2026-03-14',
      endTime: '11:00',
    });
    expect(request.starts_at).toBe('2026-03-14T09:30:00+03:00');
    expect(request.ends_at).toBe('2026-03-14T11:00:00+03:00');
  });
});

describe('eventRangeValid', () => {
  it('refuses an end that is not after the start', () => {
    const base = { ...emptyEvent('2026-03-14', DEFAULT_EVENT_COLOUR), allDay: false, startTime: '10:00', endTime: '09:00' };
    expect(eventRangeValid(base)).toBe(false);
    expect(eventRangeValid({ ...base, endTime: '10:00' })).toBe(false);
    expect(eventRangeValid({ ...base, endTime: '10:30' })).toBe(true);
  });

  it('accepts a single all-day day', () => {
    expect(eventRangeValid({ ...emptyEvent('2026-03-14', DEFAULT_EVENT_COLOUR), allDay: true })).toBe(true);
    expect(eventRangeValid({ ...emptyEvent('2026-03-14', DEFAULT_EVENT_COLOUR), allDay: true, endDate: '2026-03-13' })).toBe(false);
  });
});

describe('eventDraft', () => {
  it('shows the last day of an all-day event, not the exclusive end', () => {
    const event = {
      id: 'e-1',
      title: 'Bakım',
      all_day: true,
      starts_at: '2026-03-14T00:00:00+03:00',
      ends_at: '2026-03-16T00:00:00+03:00',
      colour: EVENT_COLOURS[1].hex,
    } as CalendarEvent;
    expect(eventDraft(event, DEFAULT_EVENT_COLOUR)).toMatchObject({ startDate: '2026-03-14', endDate: '2026-03-15', colour: EVENT_COLOURS[1].hex });
  });
});

describe('eventDays and minutesOf', () => {
  it('lists every day an event covers', () => {
    const allDay = {
      all_day: true,
      starts_at: '2026-03-14T00:00:00+03:00',
      ends_at: '2026-03-17T00:00:00+03:00',
    } as CalendarEvent;
    expect(eventDays(allDay)).toEqual(['2026-03-14', '2026-03-15', '2026-03-16']);

    const timed = {
      all_day: false,
      starts_at: '2026-03-14T09:00:00+03:00',
      ends_at: '2026-03-14T11:00:00+03:00',
    } as CalendarEvent;
    expect(eventDays(timed)).toEqual(['2026-03-14']);
    expect(minutesOf(timed.starts_at)).toBe(540);
  });
});

describe('nonWorkingDay', () => {
  it('reports the company weekend days and vacation periods (R137)', () => {
    const periods = [{ start_date: '2026-10-29', end_date: '2026-10-29', description: 'Cumhuriyet Bayramı' }];
    // 14 March 2026 is a Saturday.
    expect(nonWorkingDay('2026-03-14', [0, 6], periods).weekend).toBe(true);
    expect(nonWorkingDay('2026-03-13', [0, 6], periods).weekend).toBe(false);
    expect(nonWorkingDay('2026-10-29', [0, 6], periods).vacation?.description).toBe('Cumhuriyet Bayramı');
    expect(nonWorkingDay('2026-03-13', [5, 6], periods).weekend).toBe(true);
  });
});
