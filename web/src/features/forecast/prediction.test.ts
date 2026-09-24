import { describe, expect, it } from 'vitest';

import { dailyTotals, hoursUntilEndOf, istanbulDate, pointsOn, sumBand } from './prediction';

const p = (ts: string, m: string, lo = m, hi = m) => ({ ts, median: m, p10: lo, p90: hi });

describe('prediction helpers', () => {
  it('uses Istanbul dates', () => {
    expect(istanbulDate('2026-09-24T20:59:00Z')).toBe('2026-09-24');
    expect(istanbulDate('2026-09-24T21:00:00Z')).toBe('2026-09-25');
  });

  it('counts the hours to the end of an Istanbul day from the current hour (Q-I17)', () => {
    expect(hoursUntilEndOf('2026-09-24', Date.parse('2026-09-24T07:37:00Z'))).toBe(14); // 07:00Z → 21:00Z
    expect(hoursUntilEndOf('2026-09-25', Date.parse('2026-09-24T07:37:00Z'))).toBe(38);
  });

  it('keeps one day and sums days (Q-I12)', () => {
    const pts = [p('2026-09-24T20:00:00Z', '1.5', '1', '2'), p('2026-09-24T21:00:00Z', '2.25', '2', '3'), p('2026-09-24T22:00:00Z', '1', '0.5', '1.5')];
    expect(pointsOn(pts, '2026-09-25').map((x) => x.ts)).toEqual(['2026-09-24T21:00:00Z', '2026-09-24T22:00:00Z']);
    expect(dailyTotals(pts)).toEqual([
      { date: '2026-09-24', median: '1.50', p10: '1.00', p90: '2.00' },
      { date: '2026-09-25', median: '3.25', p10: '2.50', p90: '4.50' },
    ]);
    expect(sumBand(pts)).toEqual({ median: '4.75', p10: '3.50', p90: '6.50' });
  });
});
