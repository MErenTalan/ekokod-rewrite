import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { ConsumptionRow } from '@/lib/api/types';

import { ConsumptionPanelView } from './consumption-panel';

const month = (i: number, active: number): ConsumptionRow =>
  ({
    period_start: `2026-${String(i + 1).padStart(2, '0')}-01T00:00:00+03:00`,
    period_end: `2026-${String(i + 2).padStart(2, '0')}-01T00:00:00+03:00`,
    active_import: String(active),
    reactive_inductive_import: String(Math.round(active * 0.22)),
    reactive_capacitive_import: String(Math.round(active * 0.04)),
    partial: false,
    source: 'load_profile',
    suspect_registers: [],
  }) as ConsumptionRow;

const rows = [18400, 17250, 19100, 16800, 15900, 17400, 21200, 22450, 19800].map((v, i) => month(i, v));
const previous = rows.map((r, i) => ({ ...r, active_import: String(Number(r.active_import) * (i % 2 ? 0.9 : 1.08)) }));

const meta = {
  title: 'Features/Dashboard/ConsumptionPanel',
  component: ConsumptionPanelView,
  args: {
    rows,
    previousYear: previous,
    granularity: 'monthly' as const,
    range: { from: '2026-01-01', to: '2026-09-17' },
    today: '2026-09-17',
    onApply: () => {},
    show: { active: true, inductive: true, capacitive: false },
    onShowChange: () => {},
  },
} satisfies Meta<typeof ConsumptionPanelView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const WithoutComparison: Story = { args: { previousYear: null, granularity: 'daily' } };
export const Empty: Story = { args: { rows: [], previousYear: null } };
export const Loading: Story = { args: { loading: true } };
