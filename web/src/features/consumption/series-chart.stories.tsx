import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { ConsumptionRow } from '@/lib/api/types';

import { SeriesChartView } from './series-chart';

const rows = Array.from({ length: 14 }, (_, i) =>
  ({
    period_start: `2026-03-${String(i + 1).padStart(2, '0')}T00:00:00+03:00`,
    period_end: `2026-03-${String(i + 2).padStart(2, '0')}T00:00:00+03:00`,
    source: 'load_profile',
    active_import: String(900 + i * 37),
    reactive_inductive_import: String(220 + i * 8),
    reactive_capacitive_import: String(40 + i),
    partial: false,
    suspect_registers: [],
  }) as ConsumptionRow,
);

const meta = {
  title: 'Features/Consumption/SeriesChart',
  component: SeriesChartView,
  args: { rows, granularity: 'daily' as const, show: { active: true, inductive: true, capacitive: false }, onShowChange: () => {} },
} satisfies Meta<typeof SeriesChartView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const OnlyActive: Story = { args: { show: { active: true, inductive: false, capacitive: false } } };
export const Empty: Story = { args: { rows: [] } };
