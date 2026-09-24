import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { fn } from 'storybook/test';

import { ReportForm } from './report-form';

const meta = {
  title: 'Features/Carbon/ReportForm',
  component: ReportForm,
  args: { today: '2026-09-10', initialRange: { from: '2026-01-01', to: '2026-06-30' }, saving: false, errors: {}, onSubmit: fn() },
} satisfies Meta<typeof ReportForm>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Form: Story = {};
export const PeriodTooLong: Story = { args: { initialRange: { from: '2025-01-01', to: '2026-06-30' } } };
