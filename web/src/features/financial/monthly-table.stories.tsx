import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { monthly } from './_fixture';
import { MonthlyTable } from './monthly-table';

const meta = {
  title: 'Features/Financial/MonthlyTable',
  component: MonthlyTable,
  args: { monthly },
} satisfies Meta<typeof MonthlyTable>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Year: Story = {};
