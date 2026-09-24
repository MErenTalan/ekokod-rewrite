import { describe, expect, it } from 'vitest';

import { calculatorBody, defaultPeriod, initialValues, withChange } from './calculator-form';

describe('calculator form', () => {
  it('defaults to last month in Istanbul time', () => {
    expect(defaultPeriod('2026-09-24')).toEqual({ start: '2026-08-01', end: '2026-08-31' });
    expect(defaultPeriod('2026-01-10')).toEqual({ start: '2025-12-01', end: '2025-12-31' });
    expect(defaultPeriod('2028-03-01')).toEqual({ start: '2028-02-01', end: '2028-02-29' });
  });

  it('keeps choices the schedule can price (R351)', () => {
    const mv = withChange(initialValues('2026-09-24'), { voltage: 'mv', term: 'binomial' });
    expect(withChange(mv, { voltage: 'lv' }).term).toBe('monomial');
    const multi = withChange(mv, { multi: true });
    expect(withChange(multi, { group: 'lighting' }).multi).toBe(false);
  });

  it('sends only what the choices use', () => {
    const v = { ...initialValues('2026-09-24'), total: '1000', t1: '500', demand: '120', contract: '100' };
    expect(calculatorBody(v)).toEqual({ user_group: 'residential', voltage_level: 'lv', term: 'monomial', multi_time: false, start: '2026-08-01', end: '2026-08-31', total_consumption: '1000', t1: '500' });
    const bin = { ...v, voltage: 'mv' as const, term: 'binomial' as const };
    expect(calculatorBody(bin)).toMatchObject({ demand: '120', contract_power: '100' });
  });
});
