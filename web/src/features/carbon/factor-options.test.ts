import { describe, expect, it } from 'vitest';

import { factor } from './_fixture';
import { factorOptions, pathDetail, unitOptions } from './factor-options';

describe('factorOptions', () => {
  it('lists usable factors by label, disambiguating repeated labels with the path leaf (Q-F12)', () => {
    const list = factorOptions([
      factor({ key: 'coal_domestic', label: 'Ortam Isıtması > Kömür', category_path: ['coal', 'coal_domestic'] }),
      factor({ key: 'coal_industrial', label: 'Ortam Isıtması > Kömür', category_path: ['coal', 'coal_industrial'] }),
      factor({ key: 'lpg', label: 'Ortam Isıtması > LPG', category_path: ['lpg'] }),
      factor({ key: 'old', label: 'Eski', status: 'archived' }),
    ]);
    expect(list).toEqual([
      { value: 'coal_domestic', label: 'Ortam Isıtması > Kömür (coal domestic)' },
      { value: 'coal_industrial', label: 'Ortam Isıtması > Kömür (coal industrial)' },
      { value: 'lpg', label: 'Ortam Isıtması > LPG' },
    ]);
  });

  it('keeps a factor without a path or status', () => {
    expect(factorOptions([factor({ key: 'x', label: 'Tek', category_path: [], status: null })])).toEqual([{ value: 'x', label: 'Tek' }]);
  });
});

describe('unitOptions', () => {
  it('offers the base unit first, then the other conversions once each', () => {
    expect(unitOptions(factor())).toEqual([
      { value: 'm3', label: 'm³' },
      { value: 'kWh', label: 'kWh' },
    ]);
    expect(unitOptions(factor({ base_unit: 'tonne', conversions: [] }))).toEqual([{ value: 'tonne', label: 'tonne' }]);
  });
});

describe('pathDetail', () => {
  it('records the factor path within the 100-character detail limit', () => {
    expect(pathDetail(factor({ category_path: ['sea_freight', 'bulk', 'dwt_10k'] }))).toEqual({ path: 'sea_freight > bulk > dwt_10k' });
    expect(pathDetail(factor({ category_path: [] }))).toEqual({});
    expect(pathDetail(factor({ category_path: ['x'.repeat(120)] })).path).toHaveLength(100);
  });
});
