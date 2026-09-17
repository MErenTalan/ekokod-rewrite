import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { Analyzer, Building } from '@/lib/api/types';

import { ALL, AnalyzersTabView } from './analyzers-tab';

const analyzers = [
  { id: 'a-1', building_id: 'b-1', installation_number: '4001234567', customer_name: 'Merkez Ofis', meter_number: '56529', meter_model: 'PM5340', meter_multiplier: '1', province: 'Ankara', district: 'Çankaya', tariff_type: 'Sanayi', installed_power_kw: '250.5', is_active: true, provider: 'osos', provider_subtype: 'Baskent', activity_status: 'active', last_reading_at: '2026-09-16T23:00:00+03:00' },
  { id: 'a-2', installation_number: '4009876543', meter_multiplier: '40', is_active: true, provider: 'gridbox', provider_subtype: 'default', activity_status: 'passive' },
] as Analyzer[];
const buildings = [{ id: 'b-1', name: 'A1 Fabrika' }] as Building[];

const meta = {
  title: 'Features/Settings/AnalyzersTab',
  component: AnalyzersTabView,
  args: {
    analyzers,
    buildings,
    providers: ['osos', 'gridbox'],
    filters: { q: '', buildingId: ALL, provider: ALL },
    onFiltersChange: () => {},
    canEdit: true,
    canRefresh: true,
    onAssign: () => {},
    onRefresh: () => {},
  },
} satisfies Meta<typeof AnalyzersTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const BuildingAdmin: Story = { args: { canEdit: false } };
export const ReadOnly: Story = { args: { canEdit: false, canRefresh: false } };
export const Refreshing: Story = {
  args: { job: { id: 'job-1', label: 'Saatlik değerleri yenile', status: 'running' } },
};
