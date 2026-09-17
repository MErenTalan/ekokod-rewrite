import { describe, expect, it } from 'vitest';

import { formatPeriod } from './format-period';

describe('formatPeriod', () => {
  it('formats each granularity the way its period reads', () => {
    const ts = '2026-03-14T09:00:00+03:00';
    expect(formatPeriod(ts, 'hourly', 'tr')).toBe('14 Mar 2026 09:00');
    expect(formatPeriod(ts, 'daily', 'tr')).toBe('14 Mar 2026');
    expect(formatPeriod(ts, 'monthly', 'tr')).toBe('Mart 2026');
    expect(formatPeriod(ts, 'yearly', 'tr')).toBe('2026');
  });

  it('follows the locale for names but keeps Istanbul time', () => {
    const ts = '2026-03-14T23:00:00+03:00';
    expect(formatPeriod(ts, 'monthly', 'en')).toBe('March 2026');
    expect(formatPeriod(ts, 'hourly', 'en')).toContain('23:00');
  });
});
