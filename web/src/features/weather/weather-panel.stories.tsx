import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { WeatherPanelView } from './weather-panel';

const meta = {
  title: 'Features/Weather/Panel',
  component: WeatherPanelView,
  args: {
    weather: {
      available: true, potential_basis: 'shortwave_radiation_sum_mj_m2',
      current: { temperature_c: '21.4', humidity_pct: '55', wind_kmh: '12.3', pressure_hpa: '1012.5', visibility_km: '24.1', uv_index: '6.2', precipitation_pct: '10', weather_code: 2 },
      days: [
        { date: '2026-09-17', min_c: '15.2', max_c: '27.1', weather_code: 2, precipitation_pct: '10', uv_index_max: '6.2', shortwave_mj_m2: '21.5', potential: 'high' },
        { date: '2026-09-18', min_c: '14', max_c: '22', weather_code: 61, precipitation_pct: '80', uv_index_max: '3.1', shortwave_mj_m2: '9.9', potential: 'low' },
      ],
    },
  },
} satisfies Meta<typeof WeatherPanelView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const LocationNotConfigured: Story = { args: { weather: { available: false, reason: 'location_not_configured', days: [] } } };
export const NotConfigured: Story = { args: { weather: { available: false, reason: 'weather_not_configured', days: [] } } };
export const Loading: Story = { args: { weather: undefined, loading: true } };
