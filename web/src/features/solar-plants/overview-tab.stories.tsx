import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoDaily, demoMonthly, demoRealtime, demoRevenue } from './_fixture';
import { OverviewTabView } from './overview-tab';

const meta = {
  title: 'Features/SolarPlants/Overview',
  component: OverviewTabView,
  args: { realtime: demoRealtime, revenue: demoRevenue, monthDaily: demoDaily, history: demoMonthly },
} satisfies Meta<typeof OverviewTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Stale: Story = { args: { realtime: { ...demoRealtime, stale: true } } };
export const MissingFigures: Story = {
  args: { realtime: { ...demoRealtime, yield_month_kwh: undefined, capacity_utilisation_pct: undefined }, revenue: { available: false, reason: 'no_solar_tariff' } },
};
export const Loading: Story = { args: { realtime: undefined, revenue: undefined, monthDaily: undefined, history: undefined, loading: true } };
