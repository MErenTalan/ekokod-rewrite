import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { empty, monthly } from './_story-data';
import { ComboChart } from './combo-chart';

const meta = {
  title: 'Charts/ComboChart',
  component: ComboChart,
  args: {
    title: 'Tüketim ve güneş üretimi',
    description: 'Merkez Bina · 2026',
    xLabel: 'Ay',
    bars: [{ key: 'consumption', label: 'Tüketim', kind: 'consumption', unit: 'kWh' }],
    lines: [{ key: 'generation', label: 'Üretim', kind: 'generation', unit: 'kWh' }],
    data: monthly({ consumption: [182000, 24000], generation: [64000, 38000, -3] }),
    empty,
  },
} satisfies Meta<typeof ComboChart>;
export default meta;
type Story = StoryObj<typeof meta>;

export const ConsumptionVsGeneration: Story = {};
export const CostOnSecondAxis: Story = {
  args: {
    title: 'Tüketim ve maliyet',
    lines: [{ key: 'cost', label: 'Maliyet', kind: 'cost', unit: 'TRY' }],
    data: monthly({ consumption: [182000, 24000], cost: [412000, 60000] }),
  },
};
