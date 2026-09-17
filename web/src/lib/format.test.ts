import { describe, expect, it } from 'vitest';

import { formatBytes, formatCurrency, formatDate, formatMonth, formatNumber, formatQuantity, unitSymbol } from './format';

describe('format', () => {
  it('formats with Turkish separators', () => {
    expect(formatNumber('1234.567890', { maxFractionDigits: 2 })).toBe('1.234,57');
  });

  it('keeps full precision for decimal strings', () => {
    expect(formatNumber('12345678901234567890.123456', { maxFractionDigits: 6 })).toBe(
      '12.345.678.901.234.567.890,123456',
    );
  });

  it("default precision follows the string input, not Intl's default 3", () => {
    expect(formatNumber('1234.567891')).toBe('1.234,567891');
    expect(formatNumber('1234')).toBe('1.234');
    expect(formatNumber(1234.56789)).toBe('1.234,568');
    expect(formatNumber('1234', { minFractionDigits: 2 })).toBe('1.234,00');
  });

  it('null is an em dash, never zero', () => {
    expect(formatNumber(null)).toBe('—');
    expect(formatCurrency(undefined)).toBe('—');
    expect(formatQuantity(null, 'kWh')).toBe('—');
  });

  it('currency and units', () => {
    expect(formatCurrency('1234.56')).toBe('₺1.234,56');
    expect(formatCurrency('-1234.5')).toBe('-₺1.234,50');
    expect(formatQuantity('1234.5', 'tCO2e')).toBe('1.234,5 tCO₂e');
    expect(formatQuantity('12.5', 'percent')).toBe('%12,5');
    expect(formatQuantity('1234.5', 'TRY')).toBe('₺1.234,50');
    expect(unitSymbol('kVArh')).toBe('kVArh');
  });

  it('dates and months follow the locale, in Istanbul time', () => {
    expect(formatDate('2026-09-17', 'tr')).toBe('17 Eyl 2026');
    expect(formatDate('2026-09-17', 'en')).toBe('Sep 17, 2026');
    expect(formatMonth('2026-09-01', 'en')).toBe('September 2026');
    expect(formatMonth('2026-09', 'tr')).toBe('Eylül 2026');
    // 22:30 UTC on the 16th is already the 17th in Istanbul (UTC+3).
    expect(formatDate('2026-09-16T22:30:00Z', 'tr')).toBe('17 Eyl 2026');
  });

  it('file sizes', () => {
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(1_500_000)).toBe('1,5 MB');
    expect(formatBytes(1_000_000)).toBe('1 MB');
  });
});
