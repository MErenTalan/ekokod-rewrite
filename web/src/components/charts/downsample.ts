import type { Datum } from './_theme';

/** Largest-triangle-three-buckets on one series; keeps first and last points and visual extremes (07 §5). */
export function downsampleLttb(data: Datum[], seriesKey: string, threshold = 1000): Datum[] {
  if (threshold >= data.length || threshold < 3) return data;
  const y = (d: Datum) => Number(d[seriesKey] ?? 0);
  const x = (i: number) => i;
  const out: Datum[] = [data[0]];
  const bucket = (data.length - 2) / (threshold - 2);
  let a = 0;
  for (let i = 0; i < threshold - 2; i++) {
    const nextStart = Math.floor((i + 1) * bucket) + 1;
    const nextEnd = Math.min(Math.floor((i + 2) * bucket) + 1, data.length);
    let avgX = 0;
    let avgY = 0;
    for (let j = nextStart; j < nextEnd; j++) {
      avgX += x(j);
      avgY += y(data[j]);
    }
    const n = nextEnd - nextStart || 1;
    avgX /= n;
    avgY /= n;
    const start = Math.floor(i * bucket) + 1;
    const end = Math.floor((i + 1) * bucket) + 1;
    let maxArea = -1;
    let chosen = start;
    for (let j = start; j < end; j++) {
      const area = Math.abs((x(a) - avgX) * (y(data[j]) - y(data[a])) - (x(a) - x(j)) * (avgY - y(data[a])));
      if (area > maxArea) {
        maxArea = area;
        chosen = j;
      }
    }
    out.push(data[chosen]);
    a = chosen;
  }
  out.push(data[data.length - 1]);
  return out;
}
