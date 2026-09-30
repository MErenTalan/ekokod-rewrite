import { describe, expect, it } from 'vitest';

import { compareDecimal, fractionToPercent, shiftDecimal } from './decimal';

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

  it('shifts the decimal point both ways without floats', () => {
    expect(shiftDecimal('70', -3)).toBe('0.07');
    expect(shiftDecimal('70.5', -3)).toBe('0.0705');
    expect(shiftDecimal('1234.56', -3)).toBe('1.23456');
    expect(shiftDecimal('0.0705', 3)).toBe('70.5');
    expect(shiftDecimal('0.070000', 3)).toBe('70');
    expect(shiftDecimal('2', 3)).toBe('2000');
    expect(shiftDecimal('-1.5', 3)).toBe('-1500');
    expect(shiftDecimal('0', -3)).toBe('0');
  });
});
