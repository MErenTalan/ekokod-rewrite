import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { ConsumptionRow } from '@/lib/api/types';

import { AlarmCheckDialogView } from './alarm-check-dialog';

const row = { period_start: '2026-03-14T00:00:00+03:00', active_import: '1234.5' } as ConsumptionRow;

const meta = {
  title: 'Features/Consumption/AlarmCheckDialog',
  component: AlarmCheckDialogView,
  tags: ['open'],
  args: {
    open: true,
    onOpenChange: () => {},
    row,
    granularity: 'daily' as const,
    result: { available: false, reason: 'ml_service_unavailable' },
  },
} satisfies Meta<typeof AlarmCheckDialogView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Unavailable: Story = {};
export const Checking: Story = { args: { result: null, loading: true } };
