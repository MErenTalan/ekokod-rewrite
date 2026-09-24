import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoDaily } from './_fixture';
import { HistoryTabView } from './history-tab';

const meta = {
  title: 'Features/SolarPlants/History',
  component: HistoryTabView,
  args: {
    granularity: 'day', range: { from: '2026-09-01', to: '2026-09-17' }, series: demoDaily,
    onGranularityChange: () => {}, onRangeChange: () => {}, onExport: () => {},
  },
} satisfies Meta<typeof HistoryTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const MixedSources: Story = { args: { series: { ...demoDaily, mixed_basis: true } } };
export const RangeTooLong: Story = { args: { granularity: 'hour', range: { from: '2026-07-01', to: '2026-09-17' } } };
