import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { empty } from './_story-data';
import { GaugeChart } from './gauge-chart';

const meta = {
  title: 'Charts/GaugeChart',
  component: GaugeChart,
  args: {
    title: 'Endüktif reaktif oran',
    description: 'Merkez Bina · Ağustos 2026',
    label: 'Endüktif / aktif tüketim',
    value: '22.4',
    min: 0,
    max: 30,
    unit: 'percent',
    thresholds: [
      { value: 15, label: 'Uyarı %15', status: 'warning' },
      { value: 20, label: 'Ceza sınırı %20', status: 'danger' },
    ],
    empty,
  },
} satisfies Meta<typeof GaugeChart>;
export default meta;
type Story = StoryObj<typeof meta>;

export const ReactiveRatio: Story = {};
export const WithinLimit: Story = { args: { value: '8.25' } };
export const NoData: Story = { args: { value: null } };
export const Loading: Story = { args: { loading: true } };
