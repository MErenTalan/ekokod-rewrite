import { describe, expect, it } from 'vitest';

import { deltaDirection, formatFigure, formatMoneyList, formatRange, type ValueLabels } from './format';

const l: ValueLabels = {
  noData: 'veri yok',
  notSelected: 'seçim dışı',
  coverage: (w, o) => `${w} / ${o} bina`,
  perKwh: 'TL/kWh',
};

describe('formatFigure', () => {
  it('says "veri yok" for a missing value, never 0 (R258)', () => {
    expect(formatFigure({ value: undefined, with_data: 0, of: 2, partial: false, excluded: false }, 'kWh', l)).toBe('veri yok');
    expect(formatFigure({ value: undefined, with_data: 0, of: 2, partial: false, excluded: false }, 'kWh', l)).not.toContain('0');
  });
  it('names a figure the plant selection left out', () => {
    expect(formatFigure({ value: undefined, with_data: 0, of: 0, partial: false, excluded: true }, 'kWh', l)).toBe('seçim dışı');
  });
  it('prints the value with Turkish separators and its unit', () => {
    expect(formatFigure({ value: '1234.5', with_data: 2, of: 2, partial: false, excluded: false }, 'kWh', l)).toBe('1.234,5 kWh');
  });
  it('adds the coverage when some buildings had no data', () => {
    expect(formatFigure({ value: '1000', with_data: 1, of: 2, partial: false, excluded: false }, 'kWh', l)).toBe('1.000 kWh (1 / 2 bina)');
  });
});

describe('formatMoneyList', () => {
  it('prints one line per currency, never a sum (R270)', () => {
    expect(
      formatMoneyList(
        [
          { currency: 'TRY', value: '7500', with_data: 2, of: 2 },
          { currency: 'USD', value: '100', with_data: 1, of: 2 },
        ],
        l,
      ),
    ).toEqual(['₺7.500,00', '100,00 USD (1 / 2 bina)']);
  });
  it('says "veri yok" when there is no invoice', () => {
    expect(formatMoneyList([], l)).toEqual(['veri yok']);
  });
});

describe('formatRange', () => {
  it('prints one price when the ends meet and a range when they do not (R257)', () => {
    expect(formatRange({ min: '2.1', max: '2.1' }, l)).toBe('2,1000 TL/kWh');
    expect(formatRange({ min: '2', max: '2.5' }, l)).toBe('2,0000 – 2,5000 TL/kWh');
    expect(formatRange({}, l)).toBe('veri yok');
  });
});

describe('deltaDirection', () => {
  it('reads the sign of the percentage and has none when undefined (R259)', () => {
    expect(deltaDirection('12.5')).toBe('up');
    expect(deltaDirection('-3')).toBe('down');
    expect(deltaDirection('0')).toBe('flat');
    expect(deltaDirection(undefined)).toBeNull();
  });
});
