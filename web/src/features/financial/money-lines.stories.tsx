import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { MoneyLines } from './money-lines';

const meta = {
  title: 'Features/Financial/MoneyLines',
  component: MoneyLines,
  args: {
    list: [
      { currency: 'TRY', amount: '11150.00' },
      { currency: 'EUR', amount: '120.00' },
    ],
  },
} satisfies Meta<typeof MoneyLines>;
export default meta;
type Story = StoryObj<typeof meta>;

export const TwoCurrencies: Story = {};
export const Empty: Story = { args: { list: [] } };
