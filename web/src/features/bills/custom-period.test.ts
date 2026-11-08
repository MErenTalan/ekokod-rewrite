import { describe, expect, it } from 'vitest';

import { formatBound, keyOptions, shiftMonth, splitInstant, toInstant } from './custom-period';

describe('custom bill period', () => {
  it('sends Istanbul instants, midnight when no hour is picked', () => {
    expect(toInstant('2026-08-18', '16:00')).toBe('2026-08-18T16:00:00+03:00');
    expect(toInstant('2026-08-18', '')).toBe('2026-08-18T00:00:00+03:00');
  });

  it('offers the start month as the key, with its neighbours across a year end', () => {
    expect(keyOptions('2026-07-17')).toEqual(['2026-06', '2026-07', '2026-08']);
    expect(keyOptions('2026-01-05')).toEqual(['2025-12', '2026-01', '2026-02']);
    expect(shiftMonth('2026-12', 1)).toBe('2027-01');
  });

  it('shows the hour only when a bound is not at midnight', () => {
    expect(formatBound('2026-07-16T21:00:00Z')).toBe('17.07.2026');
    expect(formatBound('2026-07-17T13:00:00Z')).toBe('17.07.2026 16:00');
  });

  it('splits a saved bound back into the form fields', () => {
    expect(splitInstant('2026-08-18T13:00:00Z')).toEqual({ date: '2026-08-18', hour: '16:00' });
  });
});
