import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { FigureList } from './figure-list';

const meta = {
  title: 'Features/Renewable/FigureList',
  component: FigureList,
  args: {
    figures: [
      { field: 'current_power_kw', value: '42.5', unit: 'kW' },
      { field: 'status', value: 'producing', format: 'text' },
      { field: 'net_today', value: '30.5', format: 'money', currency: 'TRY' },
      { field: 'voltage_v', value: undefined, unit: 'V' },
    ],
    unavailable: { voltage_v: 'no_power_quality_measurement' },
  },
} satisfies Meta<typeof FigureList>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Mixed: Story = {};
