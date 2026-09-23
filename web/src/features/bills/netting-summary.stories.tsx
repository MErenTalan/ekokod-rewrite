import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoDashboard } from './_fixture';
import { NettingSummaryView } from './netting-summary';

const meta = {
  title: 'Features/Bills/NettingSummary',
  component: NettingSummaryView,
  args: { netting: demoDashboard.netting },
} satisfies Meta<typeof NettingSummaryView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
/** No consumption means no ratio exists; the screen says so (R165, R236). */
export const WithoutEfficiency: Story = {
  args: {
    netting: [{ ...demoDashboard.netting[0], total_consumption: '0', net: '-40', net_status: 'net_production', efficiency_pct: undefined, total_production: '40' }],
  },
};
/** R253: two currencies in one month are never added together. */
export const TwoCurrencies: Story = {
  args: {
    netting: [demoDashboard.netting[0], { ...demoDashboard.netting[0], currency: 'USD', total_invoice: '100' }],
  },
};
