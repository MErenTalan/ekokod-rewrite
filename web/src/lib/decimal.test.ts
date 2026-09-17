import { describe, expect, it } from 'vitest';

import { compareDecimal, fractionToPercent } from './decimal';

describe('decimal strings', () => {
  it('compares without floats', () => {
    expect(compareDecimal('12345678901234567.891', '12345678901234567.89')).toBe(1);
    expect(compareDecimal('-2', '-10')).toBe(1);
    expect(compareDecimal('0.10', '0.1')).toBe(0);
    expect(compareDecimal('0.2', '0.25')).toBe(-1);
  });

  it('turns a fraction into percent by moving the decimal point', () => {
    expect(fractionToPercent('0.25')).toBe('25');
    expect(fractionToPercent('0.2')).toBe('20');
    expect(fractionToPercent('0.123456')).toBe('12.3456');
    expect(fractionToPercent('1.5')).toBe('150');
    expect(fractionToPercent('-0.005')).toBe('-0.5');
    expect(fractionToPercent('3')).toBe('300');
  });
});
