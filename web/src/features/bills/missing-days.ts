/** Collapses sorted YYYY-MM-DD days into "01.09.2026" / "01.09.2026 – 05.09.2026" runs. */
export function dayRanges(days: readonly string[]): string[] {
  const fmt = (d: string) => `${d.slice(8, 10)}.${d.slice(5, 7)}.${d.slice(0, 4)}`;
  const next = (d: string) => {
    const t = new Date(`${d}T00:00:00Z`);
    t.setUTCDate(t.getUTCDate() + 1);
    return t.toISOString().slice(0, 10);
  };
  const out: string[] = [];
  let start: string | null = null;
  let prev: string | null = null;
  const flush = () => {
    if (start && prev) out.push(start === prev ? fmt(start) : `${fmt(start)} – ${fmt(prev)}`);
  };
  for (const d of days) {
    if (prev && d === next(prev)) {
      prev = d;
      continue;
    }
    flush();
    start = d;
    prev = d;
  }
  flush();
  return out;
}
