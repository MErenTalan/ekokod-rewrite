import { describe, expect, it } from 'vitest';

import { applyMode, draftErrors, draftFrom, emptyDraft, toTariffRequest } from './tariff-draft';
import type { TariffDraft } from './tariff-draft';

const filled = (over: Partial<TariffDraft> = {}): TariffDraft => ({
  ...emptyDraft(),
  effectiveFrom: '2026-09-01',
  distributionCost: '0.85',
  reactivePowerPrice: '1.2',
  vatRate: '20',
  singleTimePrice: '3.15',
  ...over,
});

describe('mode switches (R249)', () => {
  it('drops the fixed energy prices when PTF+YEKDEM is switched on', () => {
    const d = applyMode(filled({ t1Price: '1.9' }), { usePtfYekdem: true });
    expect(d.usePtfYekdem).toBe(true);
    expect(d.singleTimePrice).toBe('');
    expect(d.t1Price).toBe('');
  });

  it('keeps the power price when PTF is switched on, because a fixed power source still needs it', () => {
    // R125: with power_price_source=fixed the server requires power_unit_price
    // whether or not the energy side is dynamic.
    const d = applyMode(filled({ term: 'binomial', contractedPowerKw: '250', powerUnitPrice: '40' }), { usePtfYekdem: true });
    expect(d.powerUnitPrice).toBe('40');
    expect(d.contractedPowerKw).toBe('250');
  });

  it('drops every KBK field when PTF+YEKDEM is switched off', () => {
    const on = applyMode(filled({ kbkEnergy: '1.08', kbkT1: '1.1', useManualYekdem: true, manualYekdem: [{ year: '2026', month: '8', value: '500' }] }), { usePtfYekdem: true });
    const off = applyMode(on, { usePtfYekdem: false });
    expect(off.kbkEnergy).toBe('');
    expect(off.kbkT1).toBe('');
    expect(off.useManualYekdem).toBe(false);
    expect(off.manualYekdem).toEqual([]);
  });

  it('clears contracted power and the power price when the term becomes monomial', () => {
    // The server answers must_be_empty for both (R125), so the form must not send them.
    const d = applyMode(filled({ term: 'binomial', contractedPowerKw: '250', powerUnitPrice: '40', kbkPowerPrice: '1.1' }), { term: 'monomial' });
    expect(d.contractedPowerKw).toBe('');
    expect(d.powerUnitPrice).toBe('');
    expect(d.kbkPowerPrice).toBe('');
  });

  it('swaps the single price for the three time-of-use prices', () => {
    const multi = applyMode(filled(), { priceType: 'multi_time' });
    expect(multi.singleTimePrice).toBe('');
    const back = applyMode({ ...multi, t1Price: '1', t2Price: '2', t3Price: '3' }, { priceType: 'single_time' });
    expect([back.t1Price, back.t2Price, back.t3Price]).toEqual(['', '', '']);
  });
});

describe('toTariffRequest (R248)', () => {
  it('omits blank optional prices instead of sending "0"', () => {
    const body = toTariffRequest(filled({ overusePrice: '' }));
    expect(body).not.toHaveProperty('overuse_price');
    expect(body.single_time_price).toBe('3.15');
  });

  it('sends every price as a string, never a number', () => {
    const body = toTariffRequest(filled({ t1Price: '1.5' }));
    for (const value of Object.values(body)) {
      expect(typeof value).not.toBe('number');
    }
  });

  it('carries the taxes, extra charges and manual YEKDEM rows it was given', () => {
    const body = toTariffRequest(filled({
      taxes: [{ name: 'BTV', rate: '5' }],
      extraCharges: [{ name: 'Kapasite', basis: 'per_kwh', amount: '0.1' }],
      usePtfYekdem: true,
      kbkEnergy: '1.08',
      useManualYekdem: true,
      manualYekdem: [{ year: '2026', month: '8', value: '500' }],
    }));
    expect(body.taxes).toEqual([{ name: 'BTV', rate: '5' }]);
    expect(body.extra_charges).toEqual([{ name: 'Kapasite', basis: 'per_kwh', amount: '0.1' }]);
    expect(body.manual_yekdem).toEqual([{ year: 2026, month: 8, value: '500' }]);
  });

  it('drops a tax row the operator added and left empty', () => {
    const body = toTariffRequest(filled({ taxes: [{ name: '', rate: '' }] }));
    expect(body.taxes).toEqual([]);
  });
});

describe('draftErrors mirrors the server contract', () => {
  it('refuses a PTF tariff without an energy KBK', () => {
    // 09 §F8's acceptance criterion, checked before the request leaves.
    expect(draftErrors(filled({ usePtfYekdem: true }))).toHaveProperty('kbk_energy', 'required');
  });

  it('refuses a PTF tariff in a foreign currency (R127)', () => {
    const errors = draftErrors(filled({ usePtfYekdem: true, kbkEnergy: '1.08', currency: 'USD' }));
    expect(errors).toHaveProperty('currency', 'must_be_try');
  });

  it('asks for the three KBK coefficients on a multi-time PTF tariff', () => {
    const errors = draftErrors(filled({ usePtfYekdem: true, kbkEnergy: '1.08', priceType: 'multi_time' }));
    expect(errors).toMatchObject({ kbk_t1: 'required', kbk_t2: 'required', kbk_t3: 'required' });
  });

  it('asks for contracted power on a binomial tariff and refuses it on a monomial one', () => {
    expect(draftErrors(filled({ term: 'binomial' }))).toMatchObject({ contracted_power_kw: 'required', power_unit_price: 'required' });
    expect(draftErrors(filled({ term: 'monomial', contractedPowerKw: '250' }))).toHaveProperty('contracted_power_kw', 'must_be_empty');
  });

  it('asks for a generation price only when generation is subtracted from the total', () => {
    expect(draftErrors(filled({ generationUsage: 'subtract_from_total' }))).toHaveProperty('generation_price_per_kwh', 'required');
    expect(draftErrors(filled({ generationUsage: 'subtract_from_consumption' }))).not.toHaveProperty('generation_price_per_kwh');
  });

  it('asks for an overuse price once a daily threshold is set', () => {
    expect(draftErrors(filled({ overuseThresholdKwhPerDay: '30' }))).toHaveProperty('overuse_price', 'required');
  });

  it('requires the three fields the schema marks not-null', () => {
    const errors = draftErrors({ ...emptyDraft(), singleTimePrice: '3' });
    expect(errors).toMatchObject({ effective_from: 'required', distribution_cost: 'required', reactive_power_price: 'required', vat_rate: 'required' });
  });

  it('accepts a complete fixed-price tariff', () => {
    expect(draftErrors(filled())).toEqual({});
  });

  it('rejects a duplicated manual YEKDEM month', () => {
    const errors = draftErrors(filled({
      usePtfYekdem: true, kbkEnergy: '1.08', useManualYekdem: true,
      manualYekdem: [{ year: '2026', month: '8', value: '500' }, { year: '2026', month: '8', value: '600' }],
    }));
    expect(errors).toHaveProperty('manual_yekdem[1]', 'duplicate');
  });
});

describe('draftFrom', () => {
  it('round-trips a stored version back into the form', () => {
    // Through applyMode, as the form does: switching to PTF drops the fixed
    // energy price, so the round-trip must not bring it back.
    const body = toTariffRequest(applyMode(filled({ kbkEnergy: '1.08', taxes: [{ name: 'BTV', rate: '5' }] }), { usePtfYekdem: true }));
    const back = draftFrom({ id: 'x', created_at: '', updated_at: '', ...body } as never);
    expect(back.kbkEnergy).toBe('1.08');
    expect(back.usePtfYekdem).toBe(true);
    expect(back.taxes).toEqual([{ name: 'BTV', rate: '5' }]);
    expect(back.singleTimePrice).toBe('');
  });
});
