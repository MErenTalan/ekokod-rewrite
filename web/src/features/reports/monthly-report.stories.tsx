import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoMonthly } from './_fixture';
import { MonthlyReportView } from './monthly-report';

const meta = {
  title: 'Features/Reports/MonthlyReport',
  component: MonthlyReportView,
  args: { payload: demoMonthly },
} satisfies Meta<typeof MonthlyReportView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
/** R274: an open or unrefreshed month is labelled, never passed off as final. */
export const Partial: Story = { args: { payload: { ...demoMonthly, partial: true } } };
/** R258: missing figures say "veri yok", never 0. */
export const NoPlantProduction: Story = {
  args: { payload: { ...demoMonthly, utility: { ...demoMonthly.utility, value: undefined, with_data: 0 }, utility_feed_in: {} } },
};
/** R270: two currencies are printed apart, and the chart names the one it leaves out. */
export const TwoCurrencies: Story = {
  args: {
    payload: {
      ...demoMonthly,
      bill: [...demoMonthly.bill, { currency: 'USD', value: '1200', with_data: 1, of: 2 }],
      omitted_currencies: ['USD'],
    },
  },
};
