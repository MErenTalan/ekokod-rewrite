import { describe, expect, it } from 'vitest';

import { shiftAnchor, visibleDays, visibleRange } from './range';

describe('visibleRange', () => {
  it('draws six Monday-first weeks for a month', () => {
    // 1 March 2026 is a Sunday, so the grid starts on 23 February.
    expect(visibleRange('month', '2026-03-14')).toEqual({ from: '2026-02-23', to: '2026-04-05' });
    expect(visibleDays('month', '2026-03-14')).toHaveLength(42);
  });

  it('draws the Monday week a date falls in', () => {
    expect(visibleRange('week', '2026-03-14')).toEqual({ from: '2026-03-09', to: '2026-03-15' });
    expect(visibleRange('week', '2026-03-09')).toEqual({ from: '2026-03-09', to: '2026-03-15' });
  });

  it('draws one day, and thirty days of agenda', () => {
    expect(visibleRange('day', '2026-03-14')).toEqual({ from: '2026-03-14', to: '2026-03-14' });
    expect(visibleRange('agenda', '2026-03-14')).toEqual({ from: '2026-03-14', to: '2026-04-13' });
  });
});

describe('shiftAnchor', () => {
  it('moves months without overflowing a short one', () => {
    expect(shiftAnchor('month', '2026-01-31', 1)).toBe('2026-02-28');
    expect(shiftAnchor('month', '2026-03-31', -1)).toBe('2026-02-28');
    expect(shiftAnchor('month', '2026-12-15', 1)).toBe('2027-01-15');
  });

  it('moves a week by seven days and a day by one', () => {
    expect(shiftAnchor('week', '2026-03-14', 1)).toBe('2026-03-21');
    expect(shiftAnchor('day', '2026-03-01', -1)).toBe('2026-02-28');
    expect(shiftAnchor('agenda', '2026-03-14', 1)).toBe('2026-04-13');
  });
});
