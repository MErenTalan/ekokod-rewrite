import type { components } from '@/lib/api/schema';

export type ForecastPoint = components['schemas']['ForecastPoint'];
export type DayTotal = { date: string; median: string; p10: string; p90: string };

const HOUR = 3_600_000;
const DAY = new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Istanbul', dateStyle: 'short' });

export function istanbulDate(ts: string): string {
  return DAY.format(new Date(ts));
}

/** Istanbul is UTC+3 all year (no DST since 2016). */
function istanbulMidnight(date: string): number {
  return Date.parse(`${date}T00:00:00+03:00`);
}

/** Q-I17: hours from the current hour to the end of `date` (Istanbul). */
export function hoursUntilEndOf(date: string, now: number): number {
  const end = istanbulMidnight(date) + 24 * HOUR;
  return Math.ceil((end - Math.floor(now / HOUR) * HOUR) / HOUR);
}

export function pointsOn(points: ForecastPoint[], date: string): ForecastPoint[] {
  return points.filter((p) => istanbulDate(p.ts) === date);
}

const fixed = (n: number) => n.toFixed(2);

/** Q-I12: sums of the hourly median and of the hourly p10/p90, labelled approximate on screen. */
export function sumBand(points: ForecastPoint[]): { median: string; p10: string; p90: string } {
  let m = 0, lo = 0, hi = 0;
  for (const p of points) {
    m += Number(p.median);
    lo += Number(p.p10 ?? p.median);
    hi += Number(p.p90 ?? p.median);
  }
  return { median: fixed(m), p10: fixed(lo), p90: fixed(hi) };
}

export function dailyTotals(points: ForecastPoint[]): DayTotal[] {
  const days = new Map<string, ForecastPoint[]>();
  for (const p of points) {
    const d = istanbulDate(p.ts);
    days.set(d, [...(days.get(d) ?? []), p]);
  }
  return [...days].map(([date, pts]) => ({ date, ...sumBand(pts) }));
}
