import type { ConsumptionRow } from '@/lib/api/types';

import type { Messages } from '../../../messages';

/** Every label lives in the catalogue, so a column without a message cannot compile. */
export type ColumnLabelKey = keyof Messages['consumption']['columns'];

/**
 * The consumption table's columns, in the order 01 §7.3 lists them (R206):
 * the period, every closing index, the consumption registers with their ratios,
 * the generation registers (U1–U3 are the export tariff registers) and max
 * demand. The API's own column set is pinned by TestConsumptionColumnSet.
 */
export type ConsumptionColumn = {
  key: 'period' | keyof ConsumptionRow;
  /** Message key under `consumption.columns`. */
  labelKey: ColumnLabelKey;
  /** How the cell is rendered: a decimal, a ratio shown as a percentage, or text. */
  kind: 'period' | 'decimal' | 'ratio';
  unit?: 'kWh' | 'kVArh' | 'kW';
};

export const CONSUMPTION_COLUMNS: readonly ConsumptionColumn[] = [
  { key: 'period', labelKey: 'period', kind: 'period' },

  { key: 'active_import_index', labelKey: 'activeIndex', kind: 'decimal' },
  { key: 'reactive_inductive_import_index', labelKey: 'inductiveIndex', kind: 'decimal' },
  { key: 'reactive_capacitive_import_index', labelKey: 'capacitiveIndex', kind: 'decimal' },
  { key: 't1_import_index', labelKey: 't1Index', kind: 'decimal' },
  { key: 't2_import_index', labelKey: 't2Index', kind: 'decimal' },
  { key: 't3_import_index', labelKey: 't3Index', kind: 'decimal' },
  { key: 'active_export_index', labelKey: 'activeGenerationIndex', kind: 'decimal' },
  { key: 'reactive_inductive_export_index', labelKey: 'inductiveGenerationIndex', kind: 'decimal' },
  { key: 'reactive_capacitive_export_index', labelKey: 'capacitiveGenerationIndex', kind: 'decimal' },
  { key: 't1_export_index', labelKey: 'u1Index', kind: 'decimal' },
  { key: 't2_export_index', labelKey: 'u2Index', kind: 'decimal' },
  { key: 't3_export_index', labelKey: 'u3Index', kind: 'decimal' },

  { key: 'active_import', labelKey: 'active', kind: 'decimal', unit: 'kWh' },
  { key: 'reactive_inductive_import', labelKey: 'inductive', kind: 'decimal', unit: 'kVArh' },
  { key: 'reactive_capacitive_import', labelKey: 'capacitive', kind: 'decimal', unit: 'kVArh' },
  { key: 'inductive_ratio', labelKey: 'inductiveRatio', kind: 'ratio' },
  { key: 'capacitive_ratio', labelKey: 'capacitiveRatio', kind: 'ratio' },
  { key: 't1_import', labelKey: 't1', kind: 'decimal', unit: 'kWh' },
  { key: 't2_import', labelKey: 't2', kind: 'decimal', unit: 'kWh' },
  { key: 't3_import', labelKey: 't3', kind: 'decimal', unit: 'kWh' },

  { key: 'active_export', labelKey: 'activeGeneration', kind: 'decimal', unit: 'kWh' },
  { key: 'reactive_inductive_export', labelKey: 'inductiveGeneration', kind: 'decimal', unit: 'kVArh' },
  { key: 'reactive_capacitive_export', labelKey: 'capacitiveGeneration', kind: 'decimal', unit: 'kVArh' },
  { key: 't1_export', labelKey: 'u1', kind: 'decimal', unit: 'kWh' },
  { key: 't2_export', labelKey: 'u2', kind: 'decimal', unit: 'kWh' },
  { key: 't3_export', labelKey: 'u3', kind: 'decimal', unit: 'kWh' },

  { key: 'max_demand_kw', labelKey: 'maxDemand', kind: 'decimal', unit: 'kW' },
] as const;
