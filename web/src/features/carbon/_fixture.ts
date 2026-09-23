import type { MeResponse } from '@/lib/api/errors';
import type { CarbonActivity, CarbonCatalogue, CarbonOverview, CarbonReportSummary, EmissionFactorView } from '@/lib/api/types';

import fixture from '@/lib/session/permissions.fixture.json';

export const BUILDING = 'b-1';

export const me = (role: keyof typeof fixture): MeResponse => ({
  id: `u-${role}`,
  name: 'Ayşe Kaya',
  email: 'ayse@ornek.com.tr',
  role,
  locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'],
  session_id: 's-1',
  company: { id: 'c-1', name: 'Anadolu Tekstil' },
});

export const catalogue: CarbonCatalogue = {
  items: [
    { key: 'cat_stationary', subs: [
      { key: 'sub_space_heating', scope: 'scope_1', iso_category: 'category_1' },
      { key: 'sub_process_combustion', scope: 'scope_1', iso_category: 'category_1' },
      { key: 'sub_other_combustion', scope: 'scope_1', iso_category: 'category_1' },
    ] },
    { key: 'cat_electricity', subs: [
      { key: 'sub_grid_electricity', scope: 'scope_2', iso_category: 'category_2' },
      { key: 'sub_elec_generation', scope: 'scope_1', iso_category: 'category_1' },
    ] },
    { key: 'cat_waste', subs: [{ key: 'sub_waste_disposal', scope: 'scope_3', iso_category: 'category_6' }] },
  ],
};

export const activity = (over: Partial<CarbonActivity> = {}): CarbonActivity => ({
  id: 'a-1',
  building_id: BUILDING,
  main_category: 'cat_stationary',
  sub_category: 'sub_space_heating',
  activity_type: 'sub_space_heating',
  period_start: '2026-08-01',
  period_end: '2026-08-31',
  quantity: '1000',
  unit: 'm3',
  factor_id: 'f-gas',
  factor_key: 'natural_gas',
  factor_value: '2.06672',
  conversion_multiplier: '1',
  emission_kgco2e: '2066.72',
  scope: 'scope_1',
  iso_category: 'category_1',
  description: 'Kazan dairesi',
  details: { factor_source: 'Defra', factor_source_year: '2025' },
  status: 'pending',
  is_automated: false,
  created_by: 'u-1',
  created_at: '2026-09-01T09:00:00+03:00',
  updated_at: '2026-09-01T09:00:00+03:00',
  ...over,
});

const mains = ['cat_stationary', 'cat_mobility', 'cat_logistics', 'cat_gas_emissions', 'cat_electricity', 'cat_external_energy', 'cat_products', 'cat_waste'];

export const overview: CarbonOverview = {
  year: 2026,
  total_kgco2e: '12345.678',
  activity_count: 14,
  registered_count: 6,
  pending_count: 2,
  highest_source: { key: 'sub_space_heating', kgco2e: '8000' },
  by_category: mains.map((key, i) => ({ key, kgco2e: i === 0 ? '8000' : i === 4 ? '4345.678' : '0' })),
  by_scope: [
    { key: 'scope_1', kgco2e: '8000' },
    { key: 'scope_2', kgco2e: '4345.678' },
    { key: 'scope_3', kgco2e: '0' },
  ],
  monthly: Array.from({ length: 12 }, (_, i) => ({ month: i + 1, current_kgco2e: i < 8 ? '1000' : '0', previous_kgco2e: '900' })),
  recent: [
    activity(),
    activity({ id: 'a-2', sub_category: 'sub_grid_electricity', main_category: 'cat_electricity', scope: 'scope_2', iso_category: 'category_2',
      emission_kgco2e: '46.9', quantity: '100', unit: 'kWh', status: 'approved', is_automated: true, period_start: '2026-09-09', period_end: '2026-09-09' }),
  ],
};

export const emptyOverview: CarbonOverview = {
  ...overview,
  total_kgco2e: '0',
  activity_count: 0,
  pending_count: 0,
  highest_source: null,
  by_category: mains.map((key) => ({ key, kgco2e: '0' })),
  by_scope: overview.by_scope.map((s) => ({ ...s, kgco2e: '0' })),
  monthly: overview.monthly.map((m) => ({ ...m, current_kgco2e: '0', previous_kgco2e: '0' })),
  recent: [],
};

export const factor = (over: Partial<EmissionFactorView> = {}): EmissionFactorView => ({
  id: 'f-gas',
  key: 'natural_gas',
  label: 'Ortam Isıtması > Doğal Gaz',
  main_category: 'cat_stationary',
  sub_categories: ['sub_space_heating', 'sub_process_combustion'],
  category_path: ['natural_gas'],
  base_factor: '2.06672',
  base_unit: 'm3',
  fuel_type: null,
  vehicle_type: null,
  scope: 'scope_1',
  iso_category: 'category_1',
  status: 'active',
  source: 'Defra',
  source_year: 2025,
  source_url: 'https://www.gov.uk/',
  conversions: [
    { unit: 'm3', multiplier: '1', label: 'm³' },
    { unit: 'kWh', multiplier: '0.0948', label: 'kWh' },
  ],
  overridden: false,
  platform_base_factor: '2.06672',
  updated_at: '2026-09-01T09:00:00+03:00',
  ...over,
});

export const report = (over: Partial<CarbonReportSummary> = {}): CarbonReportSummary => ({
  id: 'r-1',
  building_id: BUILDING,
  name: 'GHG Protocol 2026-01-01 – 2026-06-30',
  report_type: 'ghg',
  period: '2026-01-01/2026-06-30',
  created_at: '2026-07-01T09:00:00+03:00',
  ...over,
});
