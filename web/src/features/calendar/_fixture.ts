import type { CalendarEvent, VacationPeriod } from '@/lib/api/types';
import { EVENT_COLOURS } from '@/styles/event-palette';

export const demoEvents: CalendarEvent[] = [
  {
    id: 'e-1',
    title: 'Bakım günü',
    all_day: true,
    starts_at: '2026-03-10T00:00:00+03:00',
    ends_at: '2026-03-11T00:00:00+03:00',
    colour: EVENT_COLOURS[0].hex,
  },
  {
    id: 'e-2',
    title: 'Enerji denetimi',
    all_day: false,
    starts_at: '2026-03-10T09:00:00+03:00',
    ends_at: '2026-03-10T11:00:00+03:00',
    colour: EVENT_COLOURS[1].hex,
  },
  {
    id: 'e-3',
    title: 'Tedarikçi toplantısı',
    all_day: false,
    starts_at: '2026-03-10T10:00:00+03:00',
    ends_at: '2026-03-10T12:00:00+03:00',
    colour: EVENT_COLOURS[2].hex,
  },
  {
    id: 'e-4',
    title: 'Sayaç okuma',
    all_day: false,
    starts_at: '2026-03-12T14:00:00+03:00',
    ends_at: '2026-03-12T15:00:00+03:00',
    colour: EVENT_COLOURS[3].hex,
  },
];

export const demoPeriods: VacationPeriod[] = [
  { id: 'v-1', start_date: '2026-03-13', end_date: '2026-03-13', description: 'Yıllık bakım tatili' },
];

export const demoWeekendDays = [0, 6];
