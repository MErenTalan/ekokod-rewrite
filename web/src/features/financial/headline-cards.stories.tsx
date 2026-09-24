import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { summary } from './_fixture';
import { HeadlineCards } from './headline-cards';

const meta = {
  title: 'Features/Financial/HeadlineCards',
  component: HeadlineCards,
  args: { summary },
} satisfies Meta<typeof HeadlineCards>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Year: Story = {};
export const MonthWithoutInvoices: Story = {
  args: {
    summary: {
      ...summary,
      month: 7,
      of: 1,
      with_data: 0,
      figures: { month: 7, cost: [], revenue: [], net: [], revenue_partial: false },
    },
  },
};
