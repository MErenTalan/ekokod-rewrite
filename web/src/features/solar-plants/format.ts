// Istanbul calendar labels for the solar screen (R161: timestamps render in Europe/Istanbul).
const TZ = 'Europe/Istanbul';

export const shortDate = (iso: string) =>
  new Intl.DateTimeFormat('tr-TR', { day: '2-digit', month: '2-digit', year: 'numeric', timeZone: TZ }).format(new Date(iso.length === 10 ? `${iso}T12:00:00Z` : iso));

export const shortDateTime = (iso: string) =>
  new Intl.DateTimeFormat('tr-TR', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit', timeZone: TZ }).format(new Date(iso));

export function pointLabel(iso: string, granularity: 'hour' | 'day' | 'month'): string {
  const d = new Date(iso);
  if (granularity === 'hour') return new Intl.DateTimeFormat('tr-TR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit', timeZone: TZ }).format(d);
  if (granularity === 'month') return new Intl.DateTimeFormat('tr-TR', { month: 'short', year: 'numeric', timeZone: TZ }).format(d);
  return new Intl.DateTimeFormat('tr-TR', { day: '2-digit', month: '2-digit', timeZone: TZ }).format(d);
}

/** R284's range limits, mirrored so the screen never asks for what the API refuses. */
export function rangeTooLong(granularity: 'hour' | 'day' | 'month', from: string, to: string): boolean {
  const days = (Date.parse(to) - Date.parse(from)) / 86_400_000;
  if (granularity === 'hour') return days > 31;
  if (granularity === 'day') return days > 366;
  const f = new Date(from);
  return Date.parse(to) > Date.UTC(f.getUTCFullYear() + 10, f.getUTCMonth(), f.getUTCDate());
}
