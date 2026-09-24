import { addDays, daysBetween, startOfWeek } from '@/lib/dates';

/** The four views 01 §7.18 asks for. */
export type CalendarView = 'month' | 'week' | 'day' | 'agenda';

const AGENDA_DAYS = 30;

/** The inclusive date range a view shows around its anchor date. */
export function visibleRange(view: CalendarView, anchor: string): { from: string; to: string } {
  switch (view) {
    case 'month': {
      const first = `${anchor.slice(0, 7)}-01`;
      const from = startOfWeek(first);
      // Six weeks always: the grid never changes height as months move.
      return { from, to: addDays(from, 41) };
    }
    case 'week': {
      const from = startOfWeek(anchor);
      return { from, to: addDays(from, 6) };
    }
    case 'day':
      return { from: anchor, to: anchor };
    default:
      return { from: anchor, to: addDays(anchor, AGENDA_DAYS) };
  }
}

/** Previous/next for a view: a month by months, a week by weeks, the rest by days. */
export function shiftAnchor(view: CalendarView, anchor: string, direction: -1 | 1): string {
  if (view === 'month') {
    const [year, month] = anchor.split('-').map(Number);
    const target = new Date(Date.UTC(year, month - 1 + direction, 1));
    const lastDay = new Date(Date.UTC(target.getUTCFullYear(), target.getUTCMonth() + 1, 0)).getUTCDate();
    const day = Math.min(Number(anchor.slice(8, 10)), lastDay);
    return `${target.toISOString().slice(0, 8)}${String(day).padStart(2, '0')}`;
  }
  if (view === 'week') return addDays(anchor, 7 * direction);
  return addDays(anchor, direction * (view === 'agenda' ? AGENDA_DAYS : 1));
}

/** The days a view draws, as ISO dates. */
export function visibleDays(view: CalendarView, anchor: string): string[] {
  const { from, to } = visibleRange(view, anchor);
  return Array.from({ length: daysBetween(from, to) + 1 }, (_, i) => addDays(from, i));
}
