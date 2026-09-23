import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { ReportSelectionView } from './report-selection';

const meta = {
  title: 'Features/Reports/ReportSelection',
  component: ReportSelectionView,
  args: {
    kind: 'monthly',
    buildings: [{ value: 'b-1', label: 'Merkez' }, { value: 'b-2', label: 'Depo' }],
    plants: [{ value: 'p-1', label: 'Arazi GES', kind: 'grid' }, { value: 'p-2', label: 'Çatı GES', kind: 'rooftop' }],
    value: { buildingIds: ['b-1'], period: '2026-08', plantSelection: 'all', plantIds: [] },
    onChange: () => {},
  },
} satisfies Meta<typeof ReportSelectionView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Monthly: Story = {};
export const Yearly: Story = { args: { kind: 'yearly', years: [2026, 2025, 2024], value: { buildingIds: ['b-1'], period: '2025', plantSelection: 'grid', plantIds: ['p-1'] } } };
/** A building admin may not list plants: no picker, the selection alone. */
export const WithoutPlants: Story = { args: { plants: null } };
