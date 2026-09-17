import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { ConsumptionRow } from '@/lib/api/types';

import { ConsumptionTableView } from './consumption-table';

const row = (i: number, overrides: Partial<ConsumptionRow> = {}): ConsumptionRow =>
  ({
    period_start: `2026-03-${String(i + 1).padStart(2, '0')}T00:00:00+03:00`,
    period_end: `2026-03-${String(i + 2).padStart(2, '0')}T00:00:00+03:00`,
    active_import: String(900 + i * 37),
    reactive_inductive_import: String(220 + i * 8),
    reactive_capacitive_import: String(40 + i),
    inductive_ratio: '0.2412',
    capacitive_ratio: '0.0441',
    max_demand_kw: String(70 + i),
    active_import_index: String(100000 + i * 900),
    t1_import: String(400 + i),
    t2_import: String(300 + i),
    t3_import: String(200 + i),
    partial: false,
    source: 'load_profile',
    suspect_registers: [],
    ...overrides,
  }) as ConsumptionRow;

const meta = {
  title: 'Features/Consumption/ConsumptionTable',
  component: ConsumptionTableView,
  args: {
    rows: Array.from({ length: 12 }, (_, i) => row(i)),
    granularity: 'daily' as const,
    canCheckAlarm: true,
    onCheckAlarm: () => {},
    onExport: () => {},
    onPrint: () => {},
  },
} satisfies Meta<typeof ConsumptionTableView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const WithQualityFlags: Story = {
  args: { rows: [row(0, { partial: true }), row(1, { suspect_registers: ['active_import'] }), row(2)] },
};
export const ReadOnly: Story = { args: { canCheckAlarm: false } };
export const Empty: Story = { args: { rows: [] } };
