import type { ReportFigure, ReportMonthly, ReportMonthPoint, ReportSummary, ReportYearly } from '@/lib/api/types';

/** Story and test data for the reports screen; never fetched (R196). */
const fig = (value: string | undefined, withData = 1, of = 1, extra: Partial<ReportFigure> = {}): ReportFigure => ({
  value, with_data: value === undefined ? 0 : withData, of, partial: false, excluded: false, ...extra,
});

const months = (current: (m: number) => string | undefined, previous: (m: number) => string | undefined): ReportMonthPoint[] =>
  Array.from({ length: 12 }, (_, i) => ({ month: i + 1, current: current(i + 1), previous: previous(i + 1) }));

export const demoMonthly: ReportMonthly = {
  year: 2026, month: 8, days_in_month: 31, plant_selection: 'all', partial: false,
  buildings: [
    { building_id: 'b-1', name: 'Merkez', tariff_name: 'Sanayi OG çift terimli', purchase_price: '3.3333', currency: 'TRY' },
    { building_id: 'b-2', name: 'Depo', tariff_name: undefined, purchase_price: undefined, currency: undefined },
  ],
  plants: [{ plant_id: 'p-1', name: 'Arazi GES', kind: 'grid', production: fig('2000'), feed_in: '2.1' }],
  consumption: fig('18450.5', 2, 2), consumption_prev: fig('17000', 2, 2), daily_consumption: fig('595.1774', 2, 2),
  t1: fig('9000'), t2: fig('6000'), t3: fig('3450.5'), inductive: fig('2100'), capacitive: fig('300'),
  rooftop: fig('320', 1, 2), rooftop_prev: fig('290', 1, 2), utility: fig('2000'), utility_prev: fig(undefined),
  production: fig('2320', 2, 3), production_prev: fig('290', 1, 3), daily_production: fig('74.8387', 2, 3),
  bill: [{ currency: 'TRY', value: '48250.75', with_data: 1, of: 2 }],
  bill_prev: [{ currency: 'TRY', value: '44100', with_data: 1, of: 2 }],
  energy_cost: [{ currency: 'TRY', value: '31800.25', with_data: 1, of: 2 }],
  distribution_cost: [{ currency: 'TRY', value: '8400.1', with_data: 1, of: 2 }],
  taxes: [{ currency: 'TRY', value: '5350.4', with_data: 1, of: 2 }],
  reactive_penalty: [{ currency: 'TRY', value: '120', with_data: 1, of: 2 }],
  reactive_penalty_prev: [{ currency: 'TRY', value: '0', with_data: 1, of: 2 }],
  inductive_ratio: '11.38', capacitive_ratio: '1.63',
  average_purchase_price: [{ currency: 'TRY', value: '1.723528', with_data: 1, of: 2 }],
  rooftop_feed_in: { min: '1.2', max: '1.2' }, utility_feed_in: { min: '2.1', max: '2.1' },
  consumption_delta: { pct: '8.5' }, production_delta: { pct: '700' },
  bill_delta: [{ currency: 'TRY', pct: '9.4' }], reactive_penalty_delta: [{ currency: 'TRY', pct: undefined }],
  consumption_chart: months((m) => (m <= 8 ? String(17000 + m * 150) : undefined), (m) => String(16000 + m * 120)),
  bill_chart: months((m) => (m <= 8 ? String(44000 + m * 530) : undefined), (m) => String(41000 + m * 400)),
  chart_currency: 'TRY', omitted_currencies: [],
};

export const demoYearly: ReportYearly = {
  year: 2025, plant_selection: 'all', partial: false,
  buildings: [{ building_id: 'b-1', name: 'Merkez', tariff_name: 'Sanayi OG çift terimli' }],
  plants: [
    { plant_id: 'p-1', name: 'Arazi GES', kind: 'grid', production: fig('108000'), target: '120000', achievement_pct: '90' },
    { plant_id: 'p-2', name: 'Kuzey GES', kind: 'grid', production: fig('50000') },
  ],
  months: Array.from({ length: 12 }, (_, i) => ({
    month: i + 1, consumption: fig('18000'), rooftop: fig('300'),
    bill: [{ currency: 'TRY', value: '48000', with_data: 1, of: 1 }], reactive_penalty: [{ currency: 'TRY', value: '0', with_data: 1, of: 1 }],
  })),
  consumption: fig('216000'), rooftop: fig('3600'), utility: fig('158000'), production: fig('161600'),
  bill: [{ currency: 'TRY', value: '576000', with_data: 1, of: 1 }], reactive_penalty: [{ currency: 'TRY', value: '0', with_data: 1, of: 1 }],
  daily_consumption: fig('591.7808'), daily_rooftop: fig('9.863'), daily_production: fig('442.7397'), daily_utility: fig('432.8767'),
  target: '120000', achievement_pct: '90',
  consumption_delta: { pct: '5.9' }, bill_delta: [{ currency: 'TRY', pct: '12.1' }],
  solar_share_pct: '74.8', grid_share_pct: '25.2',
  carbon: { factor: '0.45', factor_unit: 'kg CO2e/kWh', source_year: 2022, consumption_t: '97.2', reduction_t: '72.72', net_t: '24.48' },
  history: [
    { year: 2023, consumption: undefined, production: undefined, bill: [] },
    { year: 2024, consumption: '204000', production: '120000', bill: [{ currency: 'TRY', value: '514000', with_data: 1, of: 1 }] },
    { year: 2025, consumption: '216000', production: '161600', bill: [{ currency: 'TRY', value: '576000', with_data: 1, of: 1 }] },
  ],
  chart_currency: 'TRY', omitted_currencies: [],
};

export const demoArchive: ReportSummary[] = [
  { id: 'r-1', building_id: 'b-1', building_name: 'Merkez', type: 'yearly', period: '2025', plant_selection: 'all', status: 'completed', created_at: '2026-01-03T07:00:00+03:00', processed_at: '2026-01-03T07:01:00+03:00' },
  { id: 'r-2', building_id: 'b-1', building_name: 'Merkez', type: 'monthly', period: '2025-12', plant_selection: 'all', status: 'completed', created_at: '2026-01-02T06:00:00+03:00', processed_at: '2026-01-02T06:01:00+03:00' },
  { id: 'r-3', building_id: 'b-1', building_name: 'Merkez', type: 'monthly', period: '2025-11', plant_selection: 'all', status: 'error', created_at: '2025-12-02T06:00:00+03:00', processed_at: '2025-12-02T06:01:00+03:00' },
  { id: 'r-4', building_id: 'b-2', building_name: 'Depo', type: 'monthly', period: '2026-08', plant_selection: 'grid', status: 'pending', created_at: '2026-09-02T06:00:00+03:00', processed_at: undefined },
];
