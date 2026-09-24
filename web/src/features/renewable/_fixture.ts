import type {
  EnergyBalance,
  RenewableAnalytics,
  RenewableEfficiency,
  RenewableEnvironmental,
  RenewableForecast,
  RenewableGridInteraction,
  RenewableOverview,
  RenewableRealtime,
  RenewableSystemStatus,
} from '@/lib/api/types';

// Shapes as the API sends them: a missing figure is null with a reason (R292).
const none = null as unknown as undefined;

export const overview: RenewableOverview = {
  active_generation_kwh: '1250.5',
  inductive_generation_kvarh: '12.25',
  capacitive_generation_kvarh: '3.5',
  average_generation_kwh: '178.64',
  unavailable: {},
};

export const realtime: RenewableRealtime = {
  current_power_kw: '42.5',
  today_kwh: '210.25',
  max_power_kw: '88',
  avg_power_kw: '31.4',
  status: 'producing',
  series_24h: [
    { ts: '2026-09-22T09:00:00Z', kwh: '40' },
    { ts: '2026-09-22T10:00:00Z', kwh: '55.5' },
    { ts: '2026-09-22T11:00:00Z', kwh: none },
  ],
  system_efficiency_pct: none,
  unavailable: { system_efficiency_pct: 'no_irradiance_or_capacity_measurement' },
};

export const grid: RenewableGridInteraction = {
  direction: 'export',
  today_import_kwh: '120',
  today_export_kwh: '210.25',
  power_factor: '0.97',
  voltage_v: none,
  frequency_hz: none,
  import_price: '3.25',
  export_price: '2',
  currency: 'TRY',
  net_today: '30.5',
  unavailable: {
    voltage_v: 'no_power_quality_measurement',
    frequency_hz: 'no_power_quality_measurement',
  },
};

export const environmental: RenewableEnvironmental = {
  generation_kwh: '1250.5',
  co2_avoided_kg: '550.22',
  trees: '25.27',
  coal_kg: '646.51',
  car_km: none,
  homes: '0.5437',
  grid_factor: '0.44',
  grid_factor_unit: 'kg CO2e/kWh',
  grid_factor_source: 'TEİAŞ',
  factors: [
    {
      key: 'equiv_tree_co2_kg_per_year',
      factor: '21.77',
      unit: 'kg CO2/ağaç-yıl',
      source: 'EEA',
      year: 2023,
    },
    { key: 'equiv_coal_kg_per_kwh', factor: '0.517', unit: 'kg/kWh', source: 'EIA', year: 2022 },
    {
      key: 'equiv_home_heating_kwh_per_year',
      factor: '2300',
      unit: 'kWh/hane-yıl',
      source: 'TÜİK/EPDK',
      year: 2023,
    },
  ],
  unavailable: { car_km: 'no_factor' },
};

export const efficiency: RenewableEfficiency = {
  overall_pct: none,
  panel_pct: none,
  inverter_pct: none,
  battery_pct: none,
  grid_pct: none,
  trend: [],
  recommendations: [],
  unavailable: {
    overall_pct: 'no_irradiance_or_capacity_measurement',
    panel_pct: 'no_irradiance_or_capacity_measurement',
    inverter_pct: 'no_component_telemetry',
    battery_pct: 'no_battery_measurement',
    grid_pct: 'no_power_quality_measurement',
    trend: 'no_irradiance_or_capacity_measurement',
    recommendations: 'no_irradiance_or_capacity_measurement',
  },
};

export const forecast: RenewableForecast = {
  consumption_next_24h_kwh: '310.5',
  consumption_next_7d_kwh: '2140',
  consumption_next_28d_kwh: '8800.25',
  accuracy_daily_pct: '91.2',
  accuracy_weekly_pct: '88.75',
  accuracy_overall_pct: none,
  estimated_generation_kwh: none,
  net_excess_kwh: none,
  weather_impact: none,
  unavailable: {
    accuracy_overall_pct: 'no_overlap',
    estimated_generation_kwh: 'no_generation_forecast',
    net_excess_kwh: 'no_generation_forecast',
    weather_impact: 'no_generation_forecast',
  },
};

export const analytics: RenewableAnalytics = {
  peak_generation_kwh: '88',
  peak_generation_at: '2026-09-20T10:00:00Z',
  average_generation_kwh: '31.4',
  trend: [
    { ts: '2026-09-18T00:00:00Z', kwh: '180' },
    { ts: '2026-09-19T00:00:00Z', kwh: '195.5' },
  ],
  peak_hour: 12,
  data_availability_pct: '98.5',
  system_efficiency_pct: none,
  efficiency_change_30d_pct: none,
  consumption_optimisation: none,
  maintenance_required: none,
  financial: {
    import_price: '3.25',
    export_price: '2',
    currency: 'TRY',
    today_import_cost: '390',
    today_export_revenue: '420.5',
    net_today: '30.5',
    month_earnings: '1200',
    year_earnings: '9800.75',
    total_savings: '1200',
    roi_pct: none,
    payback_years: none,
    bill_savings: none,
    unavailable: {
      roi_pct: 'no_investment_cost',
      payback_years: 'no_investment_cost',
      bill_savings: 'no_self_consumption_measurement',
    },
  },
  unavailable: {
    system_efficiency_pct: 'no_irradiance_or_capacity_measurement',
    efficiency_change_30d_pct: 'no_irradiance_or_capacity_measurement',
    consumption_optimisation: 'no_self_consumption_measurement',
    maintenance_required: 'no_component_telemetry',
  },
};

export const systemStatus: RenewableSystemStatus = {
  overall: 'healthy',
  monitoring: 'healthy',
  grid_connection: 'healthy',
  last_reading_at: '2026-09-22T11:45:00Z',
  solar_panels: none,
  inverter: none,
  battery: none,
  security: none,
  total_generation_kwh: '1250.5',
  average_efficiency_pct: none,
  unavailable: {
    solar_panels: 'no_component_telemetry',
    inverter: 'no_component_telemetry',
    battery: 'no_battery_measurement',
    security: 'no_component_telemetry',
    average_efficiency_pct: 'no_irradiance_or_capacity_measurement',
  },
};

export const balance: EnergyBalance = {
  items: [
    {
      period_start: '2026-09-18T00:00:00+03:00',
      period_end: '2026-09-19T00:00:00+03:00',
      consumption: '300',
      generation: '180',
      grid_import: '120',
      grid_export: '0',
    },
    {
      period_start: '2026-09-19T00:00:00+03:00',
      period_end: '2026-09-20T00:00:00+03:00',
      consumption: '250',
      generation: '290',
      grid_import: '0',
      grid_export: '40',
    },
  ],
};
