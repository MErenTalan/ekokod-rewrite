import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { Building } from '@/lib/api/types';

import { BuildingsTabView } from './buildings-tab';

const buildings = [
  { id: 'b-1', name: 'A1 Fabrika', address: 'Ostim OSB, Ankara', sector: 'Üretim', bill_cutoff_day: 5, analyzer_count: 2, activity_status: 'active', created_at: '', updated_at: '' },
  { id: 'b-2', name: 'A2 Depo', address: 'Tuzla, İstanbul', sector: 'Lojistik', bill_cutoff_day: 1, analyzer_count: 1, activity_status: 'passive', created_at: '', updated_at: '' },
] as Building[];

const meta = {
  title: 'Features/Settings/BuildingsTab',
  component: BuildingsTabView,
  args: { buildings, canEdit: true, onAdd: () => {}, onEdit: () => {}, onDelete: () => {} },
} satisfies Meta<typeof BuildingsTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Editable: Story = {};
export const ReadOnly: Story = { args: { canEdit: false } };
export const Empty: Story = { args: { buildings: [] } };
