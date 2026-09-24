import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { ConsumptionRow } from '@/lib/api/types';

import { ProductionTabView } from './production-tab';

const rows = Array.from({ length: 14 }, (_, i) => ({
  period_start: `2026-09-${String(i + 1).padStart(2, '0')}T00:00:00+03:00`,
  period_end: `2026-09-${String(i + 2).padStart(2, '0')}T00:00:00+03:00`,
  active_export: i === 5 ? undefined : String(150 + ((i * 37) % 60)),
  reactive_inductive_export: String(2 + (i % 3)),
  reactive_capacitive_export: String(1 + (i % 2)),
  partial: false,
  source: 'meter',
})) as ConsumptionRow[];

const meta = {
  title: 'Features/Renewable/ProductionTab',
  component: ProductionTabView,
  args: {
    rows,
    granularity: 'daily',
    show: { active: true, inductive: true, capacitive: false },
    onShowChange: () => {},
  },
} satisfies Meta<typeof ProductionTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Daily: Story = {};
export const Empty: Story = { args: { rows: [] } };
