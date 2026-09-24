import { describe, expect, it } from 'vitest';

import { messages } from '../../../messages';

import { CONSUMPTION_COLUMNS } from './columns';

// 09 §F6 acceptance: "Consumption table shows every column listed in §7.3".
const EXPECTED = [
  'period',
  'active_import_index',
  'reactive_inductive_import_index',
  'reactive_capacitive_import_index',
  't1_import_index',
  't2_import_index',
  't3_import_index',
  'active_export_index',
  'reactive_inductive_export_index',
  'reactive_capacitive_export_index',
  't1_export_index',
  't2_export_index',
  't3_export_index',
  'active_import',
  'reactive_inductive_import',
  'reactive_capacitive_import',
  'inductive_ratio',
  'capacitive_ratio',
  't1_import',
  't2_import',
  't3_import',
  'active_export',
  'reactive_inductive_export',
  'reactive_capacitive_export',
  't1_export',
  't2_export',
  't3_export',
  'max_demand_kw',
];

describe('consumption columns', () => {
  it('lists every §7.3 column in order', () => {
    expect(CONSUMPTION_COLUMNS.map((c) => c.key)).toEqual(EXPECTED);
  });

  it('labels every column in both locales', () => {
    for (const column of CONSUMPTION_COLUMNS) {
      expect(messages.tr.consumption.columns[column.labelKey], column.key).toBeTruthy();
      expect(messages.en.consumption.columns[column.labelKey], column.key).toBeTruthy();
    }
  });

  it('shows the two ratios as ratios and the rest as decimals', () => {
    const ratios = CONSUMPTION_COLUMNS.filter((c) => c.kind === 'ratio').map((c) => c.key);
    expect(ratios).toEqual(['inductive_ratio', 'capacitive_ratio']);
  });
});
