import { describe, expect, it } from 'vitest';

import { addDays, daysBetween, formatHour, istanbulToday, monthOf, startOfWeek } from './dates';

describe('dates', () => {
  it('reads today in Istanbul, not in the browser zone', () => {
    // 22:30 UTC is already the next day in Istanbul (UTC+3).
    expect(istanbulToday(new Date('2026-03-14T22:30:00Z'))).toBe('2026-03-15');
    expect(istanbulToday(new Date('2026-03-14T09:00:00Z'))).toBe('2026-03-14');
  });

  it('shifts dates over month and year ends', () => {
    expect(addDays('2026-02-28', 1)).toBe('2026-03-01');
    expect(addDays('2024-02-28', 1)).toBe('2024-02-29');
    expect(addDays('2026-01-01', -1)).toBe('2025-12-31');
    expect(daysBetween('2026-03-10', '2026-03-17')).toBe(7);
  });

  it('starts weeks on Monday', () => {
    expect(startOfWeek('2026-03-08')).toBe('2026-03-02'); // Sunday
    expect(startOfWeek('2026-03-09')).toBe('2026-03-09'); // Monday
    expect(startOfWeek('2026-03-15')).toBe('2026-03-09');
  });

  it('formats hours and months', () => {
    expect(formatHour(0)).toBe('00:00');
    expect(formatHour(9)).toBe('09:00');
    expect(formatHour(23)).toBe('23:00');
    expect(monthOf('2026-03-14')).toBe('2026-03');
  });
});
