import { describe, expect, it } from 'vitest';

import type { Weather } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { WeatherPanelView } from './weather-panel';

const weather: Weather = {
  available: true,
  potential_basis: 'shortwave_radiation_sum_mj_m2',
  current: { temperature_c: '21.4', humidity_pct: '55', wind_kmh: '12.3', pressure_hpa: '1012.5', visibility_km: '24.14', uv_index: '6.2', precipitation_pct: '10', weather_code: 2 },
  days: [
    { date: '2026-09-17', min_c: '15.2', max_c: '27.1', weather_code: 2, precipitation_pct: '10', uv_index_max: '6.2', shortwave_mj_m2: '21.5', potential: 'high' },
    { date: '2026-09-18', min_c: '14', max_c: undefined, weather_code: 61, precipitation_pct: '80', uv_index_max: '3.1', shortwave_mj_m2: '9.9', potential: 'low' },
  ],
};

describe('WeatherPanelView', () => {
  it('shows current conditions with units (R291)', () => {
    const r = renderWithProviders(<WeatherPanelView weather={weather} />);
    expect(r.getByText('21,4')).toBeVisible();
    expect(r.getAllByText(/Parçalı bulutlu/).length).toBe(2); // now and today
    expect(r.getByText(/24,14 km/)).toBeVisible();
  });

  it('lists the forecast days with their generation potential', () => {
    const r = renderWithProviders(<WeatherPanelView weather={weather} />);
    expect(r.getByText(/Yüksek/)).toBeVisible();
    expect(r.getByText(/Düşük/)).toBeVisible();
    expect(r.getByText(/Yağmurlu/)).toBeVisible();
  });

  it('says "konum ayarlı değil" and names no city when there are no coordinates', () => {
    const r = renderWithProviders(<WeatherPanelView weather={{ available: false, reason: 'location_not_configured', days: [] }} />);
    expect(r.getByText(/Konum ayarlı değil/)).toBeVisible();
    expect(r.queryByText(/Ankara|Türkiye/)).toBeNull();
  });

  it('says weather is not configured in an air-gapped installation', () => {
    const r = renderWithProviders(<WeatherPanelView weather={{ available: false, reason: 'weather_not_configured', days: [] }} />);
    expect(r.getByText(/Hava durumu hizmeti yapılandırılmamış/)).toBeVisible();
  });
});
