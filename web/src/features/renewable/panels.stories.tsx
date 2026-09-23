import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import {
  analytics,
  efficiency,
  environmental,
  forecast,
  grid,
  realtime,
  systemStatus,
} from './_fixture';
import {
  AnalyticsPanelView,
  EfficiencyPanelView,
  EnvironmentalPanelView,
  ForecastPanelView,
  GridPanelView,
  RealtimePanelView,
  SystemStatusPanelView,
} from './panels';

const meta = {
  title: 'Features/Renewable/Panels',
  component: RealtimePanelView,
  args: { data: realtime },
} satisfies Meta<typeof RealtimePanelView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Realtime: Story = {};
export const Loading: Story = { args: { data: undefined, loading: true } };
export const Failed: Story = { args: { data: undefined } };
export const Grid: Story = { render: () => <GridPanelView data={grid} /> };
export const Environmental: Story = {
  render: () => <EnvironmentalPanelView data={environmental} />,
};
export const Efficiency: Story = { render: () => <EfficiencyPanelView data={efficiency} /> };
export const Forecast: Story = { render: () => <ForecastPanelView data={forecast} /> };
export const Analytics: Story = { render: () => <AnalyticsPanelView data={analytics} /> };
export const SystemStatus: Story = { render: () => <SystemStatusPanelView data={systemStatus} /> };
