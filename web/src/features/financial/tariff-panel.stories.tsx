import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { summary } from './_fixture';
import { TariffPanel } from './tariff-panel';

const meta = {
  title: 'Features/Financial/TariffPanel',
  component: TariffPanel,
  args: { tariffs: summary.tariffs },
} satisfies Meta<typeof TariffPanel>;
export default meta;
type Story = StoryObj<typeof meta>;

export const InForce: Story = {};
export const Missing: Story = {
  args: { tariffs: { purchase: [], sale: [], purchase_missing: true, sale_missing: true } },
};
