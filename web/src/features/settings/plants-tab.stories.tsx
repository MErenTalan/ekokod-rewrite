import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { Plant } from '@/lib/api/types';

import { PlantsTabView } from './plants-tab';

const plants = [
  { id: 'p-1', name: 'Çatı GES', plant_kind: 'rooftop', installation_number: '400123', total_capacity_kw: '250.00', isolar_ps_name: 'PS-1024', created_at: '' },
  { id: 'p-2', name: 'Saha GES', plant_kind: 'grid', installation_number: '400124', total_capacity_kw: '1200.00', created_at: '' },
] as Plant[];

const meta = {
  title: 'Features/Settings/PlantsTab',
  component: PlantsTabView,
  args: { plants, canEdit: true, onAdd: () => {}, onEdit: () => {}, onDelete: () => {} },
} satisfies Meta<typeof PlantsTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Editable: Story = {};
export const ReadOnly: Story = { args: { canEdit: false } };
export const Empty: Story = { args: { plants: [] } };
