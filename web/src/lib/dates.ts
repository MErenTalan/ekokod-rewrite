// Calendar dates are ISO strings evaluated in Istanbul (plan D19, R197); no
// component ever holds a Date.
const ISTANBUL = 'Europe/Istanbul';

/** Today in Istanbul as 'YYYY-MM-DD', whatever the browser's own zone is. */
export function istanbulToday(now: Date = new Date()): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: ISTANBUL, dateStyle: 'short' }).format(now);
}

const utc = (iso: string) => {
  const [y, m, d] = iso.split('-').map(Number);
  return Date.UTC(y, m - 1, d);
};

/** Shifts an ISO date by whole days; UTC arithmetic, so no zone can move it. */
export function addDays(iso: string, days: number): string {
  return new Date(utc(iso) + days * 86_400_000).toISOString().slice(0, 10);
}

/** Whole days from `from` to `to`, both ISO dates, inclusive of neither end. */
export function daysBetween(from: string, to: string): number {
  return Math.round((utc(to) - utc(from)) / 86_400_000);
}

/** The Monday of the ISO week an ISO date falls in (weeks start Monday, D19). */
export function startOfWeek(iso: string): string {
  const weekday = new Date(utc(iso)).getUTCDay(); // 0 = Sunday
  return addDays(iso, -((weekday + 6) % 7));
}

/** 9 → '09:00', for the 24-hour axes of the load profile. */
export function formatHour(hour: number): string {
  return `${String(hour).padStart(2, '0')}:00`;
}

/** 'YYYY-MM' of an ISO date, for month pickers and bill periods. */
export function monthOf(iso: string): string {
  return iso.slice(0, 7);
}
