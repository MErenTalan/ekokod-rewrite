import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { fn } from 'storybook/test';

import { activity } from './_fixture';
import { StatusTable } from './status-table';

const rows = [
  activity(),
  activity({ id: 'a-2', sub_category: 'sub_grid_electricity', scope: 'scope_2', iso_category: 'category_2', status: 'approved', is_automated: true,
    quantity: '100', unit: 'kWh', emission_kgco2e: '46.9', period_start: '2026-09-09', period_end: '2026-09-09' }),
  activity({ id: 'a-3', status: 'rejected', sub_category: 'sub_waste_disposal', scope: 'scope_3', iso_category: 'category_6', quantity: '2.5', unit: 'tonne' }),
];

const meta = {
  title: 'Features/Carbon/StatusTable',
  component: StatusTable,
  args: {
    rows, editable: true, filters: { status: 'all', scope: 'all' }, onFilters: fn(), onApprove: fn(), onReject: fn(),
    onEdit: fn(), onDelete: fn(), hasMore: false, onLoadMore: fn(),
  },
} satisfies Meta<typeof StatusTable>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Mixed: Story = {};
export const ReadOnly: Story = { args: { editable: false } };
export const Empty: Story = { args: { rows: [] } };
export const MorePages: Story = { args: { hasMore: true } };
