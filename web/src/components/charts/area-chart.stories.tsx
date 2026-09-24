import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { empty, wave } from './_story-data';
import { AreaChart } from './area-chart';

const meta = {
  title: 'Charts/AreaChart',
  component: AreaChart,
  args: {
    title: 'Günlük güneş üretimi',
    description: 'GES Sahası · Eylül 2026',
    xLabel: 'Gün',
    series: [{ key: 'gen', label: 'Üretim', kind: 'generation', unit: 'kWh' }],
    data: Array.from({ length: 30 }, (_, d) => ({ x: `${d + 1} Eyl`, gen: wave(d, 5400, 900, 7) })),
    empty,
  },
} satisfies Meta<typeof AreaChart>;
export default meta;
type Story = StoryObj<typeof meta>;

export const SingleSeries: Story = {};
export const Empty: Story = { args: { data: [] } };
