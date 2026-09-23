import type { Tariff, TariffSummaryItem } from '@/lib/api/types';

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
