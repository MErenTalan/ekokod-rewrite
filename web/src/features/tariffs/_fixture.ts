import type {
  BuildingTariffState, BulkTariffAssignment, IcmalImport, NationalTariff, SolarTariff,
  Tariff, TariffSummaryItem, TariffTemplate,
} from '@/lib/api/types';

/** Story and test data for the tariff screens; never fetched (R196). */
export const demoTariffSummaries: TariffSummaryItem[] = [
  { id: 't-2', building_id: 'b-1', name: 'PTF geçişi', effective_from: '2026-07-01', price_type: 'single_time', term: 'binomial', use_ptf_yekdem: true },
  { id: 't-1', building_id: 'b-1', name: 'Kuruluş tarifesi', effective_from: '2025-01-01', price_type: 'multi_time', term: 'monomial', use_ptf_yekdem: false },
];

export const demoTariff: Tariff = {
  id: 't-2',
  building_id: 'b-1',
  name: 'PTF geçişi',
  effective_from: '2026-07-01',
  currency: 'TRY',
  energy_type: 'grid_energy',
  voltage_level: 'mv',
  user_group: 'industrial',
  price_type: 'single_time',
  term: 'binomial',
  supply_company: 'private',
  generation_usage: 'none',
  distribution_cost: '0.850000',
  reactive_power_price: '1.200000',
  vat_rate: '20',
  use_ptf_yekdem: true,
  kbk_energy: '1.080000',
  kbk_distribution_cost_tl_per_kwh: '0.850000',
  kbk_reactive_power: '1.200000',
  contracted_power_kw: '250.000',
  power_unit_price: '40.000000',
  power_price_source: 'fixed',
  reactive_price_source: 'kbk',
  distribution_price_source: 'kbk',
  use_manual_yekdem: false,
  taxes: [{ name: 'BTV', rate: '5' }],
  extra_charges: [],
  manual_yekdem: [],
  created_at: '2026-06-20T08:00:00Z',
  updated_at: '2026-06-20T08:00:00Z',
};

export const demoTemplates: TariffTemplate[] = [
  {
    id: 'tpl-1',
    name: 'Ticari AG',
    description: 'Şehir içi ticarethaneler',
    is_default: true,
    tariff: { ...demoTariff, use_ptf_yekdem: false, kbk_energy: undefined },
    created_at: '2026-05-01T08:00:00Z',
    updated_at: '2026-05-01T08:00:00Z',
  },
];

export const demoBuildingStates: BuildingTariffState[] = [
  { building_id: 'b-1', building_name: 'A1 Fabrika', tariff_id: 't-2', tariff_name: 'PTF geçişi', effective_from: '2026-07-01', use_ptf_yekdem: true },
  { building_id: 'b-2', building_name: 'A2 Depo', use_ptf_yekdem: false },
];

export const demoAssignments: BulkTariffAssignment[] = [
  {
    id: 'ba-1', template_id: 'tpl-1', tariff_name: 'Ticari AG', effective_from: '2026-09-01',
    building_ids: ['b-1', 'b-2'], created_by: 'u-1', created_at: '2026-09-01T09:00:00Z',
  },
  {
    id: 'ba-2', tariff_name: 'Elle atanan', effective_from: '2026-06-01',
    building_ids: ['b-1'], created_at: '2026-06-01T09:00:00Z',
  },
];

export const demoIcmalImport: IcmalImport = {
  id: 'imp-1',
  file_name: 'icmal.csv',
  status: 'analysed',
  row_count: 24,
  unmatched: ['40Z0000000123A'],
  warnings: [],
  created_at: '2026-09-20T10:00:00Z',
  analyses: [
    {
      etso_code: '40ZTEST000000030',
      building_id: 'b-1',
      periods: ['202511', '202512'],
      energy_kbk: { value: '1.0800', samples: 6, std_dev: '0.0100', stable: true, back_calc_error_pct: '0.4' },
      distribution_tl_per_kwh: { value: '0.8500', samples: 6, stable: true },
      power_unit_price: { value: '43.3700', samples: 2, stable: false },
      implied_contracted_power_kw: '400',
      reactive_unit_price: { value: '3.4900', samples: 6, stable: true },
      reactive_kbk: { value: '1.1000', samples: 6, stable: true },
      vat_rate: { value: '20', samples: 6, stable: true },
      taxes: { BTV: { value: '5', samples: 6, stable: true } },
      is_multi_time: false,
      term: 'binomial',
      voltage_level: 'mv',
      within_tolerance: true,
      overuse_requires_manual_entry: true,
      warnings: [{ row: 4, code: 'power_unstable', text: 'güç fiyatı dönemler arasında kararsız' }],
    },
  ],
};

export const demoSolarTariffs: SolarTariff[] = [
  { id: 'st-1', plant_id: 'p-1', effective_from: '2026-01-01', feed_in_tariff: '2.500000', purchase_price: '3.100000', currency: 'TRY', notes: 'YEKDEM', created_at: '2026-01-01T00:00:00Z' },
];

export const demoNationalTariffs: NationalTariff[] = [
  {
    id: 'nt-1', effective_from: '2026-01-01', user_group: 'commercial', voltage_level: 'lv', term: 'monomial',
    energy_price: '3.100000', distribution_price: '2.400000', vat_rate: '20', source: 'EPDK',
    created_at: '2026-01-01T00:00:00Z',
  },
];
