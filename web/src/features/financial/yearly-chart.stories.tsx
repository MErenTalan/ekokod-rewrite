import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { monthly } from './_fixture';
import { YearlyChart } from './yearly-chart';

const meta = {
  title: 'Features/Financial/YearlyChart',
  component: YearlyChart,
  args: { monthly },
} satisfies Meta<typeof YearlyChart>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Year: Story = {};
export const Loading: Story = { args: { monthly: undefined, loading: true } };
