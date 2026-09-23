import type { Plant, PlantDeviceView, PlantFaultPage, PlantProductionSeries, PlantRealtime, PlantRevenue } from '@/lib/api/types';

/** Story and test data for the solar screen; never fetched (R196). */
export const demoPlants: Plant[] = [
  {
    id: 'p-1', name: 'Konya GES', plant_kind: 'grid', isolar_ps_id: 'PS-1', isolar_ps_name: 'Konya Solar',
    isolar_installed_kw: '250', total_capacity_kw: '250', isolar_last_sync_at: '2026-09-17T09:50:00Z', created_at: '2026-01-01T00:00:00Z',
  } as Plant,
  { id: 'p-2', name: 'Çatı GES', plant_kind: 'rooftop', isolar_ps_id: 'PS-2', created_at: '2026-01-01T00:00:00Z' } as Plant,
  { id: 'p-3', name: 'Bağlı olmayan', plant_kind: 'rooftop', created_at: '2026-01-01T00:00:00Z' } as Plant,
];

export const demoRealtime: PlantRealtime = {
  as_of: '2026-09-17T09:45:00Z', stale: false, inverter_count: 2, active_power_kw: '125.4', yield_today_kwh: '812.5',
  yield_month_kwh: '14250', yield_year_kwh: '301200', yield_total_kwh: '1250000', capacity_kw: '250',
  capacity_utilisation_pct: '50.2', connection: 'connected', last_sync_at: '2026-09-17T09:50:00Z',
};

export const demoRevenue: PlantRevenue = {
  available: true,
  daily: { amounts: [{ currency: 'TRY', amount: '1625.00' }], unpriced_days: 0, partial: false },
  monthly: { amounts: [{ currency: 'TRY', amount: '28500.00' }], unpriced_days: 0, partial: false },
  yearly: { amounts: [{ currency: 'TRY', amount: '602400.00' }], unpriced_days: 3, partial: true },
  total: { amounts: [{ currency: 'TRY', amount: '602400.00' }], unpriced_days: 3, partial: true, since: '2026-01-01' },
};

export const demoDaily: PlantProductionSeries = {
  granularity: 'day', mixed_basis: false,
  points: [
    { ts: '2026-09-14T21:00:00Z', production_kwh: '1502.4', basis: 'daily_total' },
    { ts: '2026-09-15T21:00:00Z', production_kwh: '1420.8', basis: 'plant_meter' },
    { ts: '2026-09-16T21:00:00Z', production_kwh: '812.5', basis: 'plant_meter' },
  ],
};

export const demoMonthly: PlantProductionSeries = {
  granularity: 'month', mixed_basis: false,
  points: [
    { ts: '2026-07-31T21:00:00Z', production_kwh: '42100' },
    { ts: '2026-08-31T21:00:00Z', production_kwh: '14250' },
  ],
};

export const demoDevices: PlantDeviceView[] = [
  { id: 'd-1', device_sn: 'A2103456789', device_name: 'İnverter 1', device_type: 1, status: 'normal', active_power_kw: '62.1',
    yield_today_kwh: '401.2', yield_total_kwh: '620000', last_update: '2026-09-17T09:45:00Z' },
  { id: 'd-2', device_sn: 'B9988776655', device_name: 'İnverter 2', device_type: 1, status: 'fault', active_power_kw: '0',
    yield_today_kwh: '0', yield_total_kwh: '630000', last_update: '2026-09-17T06:45:00Z' },
];

export const demoAlarms: PlantFaultPage = {
  total: 2,
  items: [
    { ref: 'r-1', code: '10', name: '电网掉电', message_tr: 'İnverter 1 cihazında şebeke kesintisi hatası oluştu.', translated: true,
      level: 1, type: 1, device_name: 'İnverter 1', occurred_at: '2026-09-17T06:00:00Z', closed_at: '2026-09-17T07:00:00Z' },
    { ref: 'r-2', code: '999', name: 'Grid overvoltage', message_tr: 'İnverter 2 cihazında hata: Grid overvoltage', translated: false,
      level: 4, type: 2, device_name: 'İnverter 2', occurred_at: '2026-09-16T12:00:00Z' },
  ],
};
