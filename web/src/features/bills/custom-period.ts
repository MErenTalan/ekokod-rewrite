// A custom bill window (F30, R554–R559): Istanbul calendar dates plus an
// optional whole hour, sent to the API as instants with Istanbul's offset.

/** Istanbul has kept UTC+3 all year since 2016. */
const ISTANBUL_OFFSET = '+03:00';

export const HOURS: string[] = Array.from({ length: 24 }, (_, h) => `${String(h).padStart(2, '0')}:00`);

/** '2026-08-18' + '16:00' → '2026-08-18T16:00:00+03:00'; an empty hour is midnight. */
export function toInstant(date: string, hour: string): string {
  return `${date}T${hour || '00:00'}:00${ISTANBUL_OFFSET}`;
}

/** 'YYYY-MM' shifted by whole months. */
export function shiftMonth(key: string, delta: number): string {
  const [y, m] = key.split('-').map(Number);
  const index = y * 12 + (m - 1) + delta;
  return `${Math.floor(index / 12)}-${String((index % 12) + 1).padStart(2, '0')}`;
}

/**
 * The period keys a window starting on `fromDate` may be saved under: the
 * start month by default (R554), or the month before or after it.
 */
export function keyOptions(fromDate: string): string[] {
  const key = fromDate.slice(0, 7);
  return [shiftMonth(key, -1), key, shiftMonth(key, 1)];
}

const istanbulParts = (iso: string) =>
  Object.fromEntries(
    new Intl.DateTimeFormat('en-GB', {
      timeZone: 'Europe/Istanbul', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
    })
      .formatToParts(new Date(iso))
      .map((p) => [p.type, p.value]),
  ) as Record<string, string>;

/** '17.07.2026' at midnight, '17.07.2026 16:00' otherwise: hours only when they matter. */
export function formatBound(iso: string): string {
  const p = istanbulParts(iso);
  const date = `${p.day}.${p.month}.${p.year}`;
  return p.hour === '00' && p.minute === '00' ? date : `${date} ${p.hour}:${p.minute}`;
}

/** Splits an instant into the form's Istanbul date and hour. */
export function splitInstant(iso: string): { date: string; hour: string } {
  const p = istanbulParts(iso);
  return { date: `${p.year}-${p.month}-${p.day}`, hour: `${p.hour}:${p.minute}` };
}

type LineTranslator = (key: string) => string;

/** A line code in the operator's language; 'extra:Kapasite' → 'Ek bedel: Kapasite' (codes are labels, R560). */
export function lineLabel(code: string, t: LineTranslator): string {
  const [prefix, name] = code.split(/:(.*)/s);
  if (name !== undefined && (prefix === 'extra' || prefix === 'tax')) return `${t(prefix)}: ${name}`;
  return t(code);
}
