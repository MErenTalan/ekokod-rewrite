import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { ReactivePanelView, type ReactiveRow } from './reactive-panel';

const rows: ReactiveRow[] = [
  {
    analyzer_id: 'a-1',
    building_id: 'b-1',
    analyzerLabel: 'Merkez Ofis Analizör',
    buildingName: 'A1 Fabrika',
    has_data: true,
    inductive_ratio: '0.3512',
    capacitive_ratio: '0.0421',
    inductive_limit: '0.20',
    capacitive_limit: '0.15',
    installed_power_kw: '250.5',
    penalty_applies: true,
  },
  {
    analyzer_id: 'a-2',
    building_id: 'b-2',
    analyzerLabel: '4009876543',
    buildingName: 'A2 Depo',
    has_data: true,
    inductive_ratio: '0.1102',
    capacitive_ratio: '0.0180',
    inductive_limit: '0.20',
    capacitive_limit: '0.15',
    installed_power_kw: '8',
    penalty_applies: false,
    exempt_reason: 'below_kw',
  },
];

const meta = {
  title: 'Features/Dashboard/ReactivePanel',
  component: ReactivePanelView,
  args: {
    month: '2026-09',
    maxMonth: '2026-09',
    onMonthChange: () => {},
    scope: 'all',
    scopeOptions: [
      { value: 'all', label: 'Tüm binalar' },
      { value: 'b-1', label: 'A1 Fabrika' },
    ],
    onScopeChange: () => {},
    rows,
    highestInductive: rows[0],
    highestCapacitive: rows[0],
  },
} satisfies Meta<typeof ReactivePanelView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const OverLimit: Story = {};
export const WithinLimits: Story = {
  args: { rows: [rows[1]], highestInductive: rows[1], highestCapacitive: rows[1] },
};
export const NoData: Story = { args: { rows: [], highestInductive: undefined, highestCapacitive: undefined } };
export const Loading: Story = { args: { loading: true } };
