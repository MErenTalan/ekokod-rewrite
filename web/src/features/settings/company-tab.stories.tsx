import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { CompanyDetail } from '@/lib/api/types';

import { CompanyTabView } from './company-tab';

const company: CompanyDetail = {
  id: 'c-1',
  name: 'Anadolu Tekstil A.Ş.',
  address: 'Ostim OSB, Ankara',
  sector: 'Üretim',
  total_area_m2: '5000.00',
  personnel_count: 120,
  contact_name: 'Ayşe Kaya',
  contact_phone: '0312 000 00 00',
  analyzer_counts: [
    { provider: 'osos', subtype: 'Baskent', count: 2 },
    { provider: 'gridbox', subtype: 'default', count: 1 },
  ],
  created_at: '',
  updated_at: '',
};

const meta = {
  title: 'Features/Settings/CompanyTab',
  component: CompanyTabView,
  args: { company, canEdit: true, onSave: () => {} },
} satisfies Meta<typeof CompanyTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Editable: Story = {};
export const ReadOnly: Story = { args: { canEdit: false } };
export const NoAnalyzers: Story = { args: { company: { ...company, analyzer_counts: [] } } };
