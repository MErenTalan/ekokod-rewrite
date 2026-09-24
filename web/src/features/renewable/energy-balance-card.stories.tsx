import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { balance } from './_fixture';
import { EnergyBalanceCard } from './energy-balance-card';

const meta = {
  title: 'Features/Renewable/EnergyBalanceCard',
  component: EnergyBalanceCard,
  args: { data: balance, granularity: 'daily' },
} satisfies Meta<typeof EnergyBalanceCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Daily: Story = {};
export const Loading: Story = { args: { data: undefined, loading: true } };
