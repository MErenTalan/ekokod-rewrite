import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { ConsumptionGrouped } from '@/lib/api/types';

import { DetailedGraphsView } from './detailed-graphs';

const group = (key: string, active: number) => ({
  key,
  days: 7,
  active_import: String(active),
  reactive_inductive_import: String(Math.round(active * 0.22)),
  reactive_capacitive_import: String(Math.round(active * 0.04)),
  partial: false,
});

const data: ConsumptionGrouped = {
  group_by: 'day_type',
  current: {
    from: '2026-03-01',
    to: '2026-03-31',
    groups: [group('weekday', 14200), group('weekend', 4100)],
    statistics: {
      total: '18300',
      average: '9150',
      peak: { key: 'weekday', value: '14200' },
      valley: { key: 'weekend', value: '4100' },
    },
  },
  previous: {
    from: '2026-01-29',
    to: '2026-02-28',
    groups: [group('weekday', 13100), group('weekend', 3900)],
    statistics: { total: '17000', average: '8500' },
  },
};

const meta = {
  title: 'Features/Consumption/DetailedGraphs',
  component: DetailedGraphsView,
  args: {
    data,
    groupBy: 'day_type' as const,
    onGroupByChange: () => {},
    compare: false,
    onCompareChange: () => {},
    show: { active: true, inductive: false, capacitive: false },
    onShowChange: () => {},
  },
} satisfies Meta<typeof DetailedGraphsView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const ByDayType: Story = {};
export const WithComparison: Story = { args: { compare: true } };
export const Empty: Story = { args: { data: null } };
