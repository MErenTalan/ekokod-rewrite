import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { actuals, forecast } from './_fixture';
import { ForecastView } from './forecast-view';

const meta = {
  title: 'Features/Forecast/ForecastView',
  component: ForecastView,
  args: { actuals, forecast, unavailable: false, loading: false },
} satisfies Meta<typeof ForecastView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const WithBandAndGaps: Story = {};
export const Fallback: Story = { args: { forecast: { ...forecast, model_id: 'seasonal_naive', fallback_from: 'random_forest', gaps: [] } } };
export const NoStoredRun: Story = { args: { forecast: null } };
export const InsufficientData: Story = { args: { forecast: { ...forecast, status: 'insufficient_data', points: [] } } };
export const ServiceDown: Story = { args: { unavailable: true } };
export const Loading: Story = { args: { loading: true } };
