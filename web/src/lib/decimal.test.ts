import { describe, expect, it } from 'vitest';

import { compareDecimal, fractionToPercent, isDecimalText, shiftDecimal, toDecimalString } from './decimal';

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

describe('toDecimalString', () => {
  it('reads a Turkish comma and keeps a lone dot as the decimal point', () => {
    expect(toDecimalString('2,45')).toBe('2.45');
    expect(toDecimalString(' 1.234,56 ')).toBe('1234.56');
    expect(toDecimalString('1,234.5')).toBe('1234.5');
    expect(toDecimalString('2.450')).toBe('2.450');
    expect(toDecimalString('-0,5')).toBe('-0.5');
    expect(toDecimalString('20')).toBe('20');
  });

  it('hands text that is no number back for the API to name', () => {
    expect(toDecimalString('2,4,5')).toBe('2,4,5');
    expect(toDecimalString(' abc ')).toBe('abc');
    expect(isDecimalText('2,45')).toBe(true);
    expect(isDecimalText('1,2,3')).toBe(false);
    expect(isDecimalText('')).toBe(false);
  });
});
