import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { CompanyView } from './company-view';

const company = {
  id: 'c-1', name: 'Anadolu Tekstil', address: 'Organize Sanayi 3. Cad. No 7, Bursa', sector: 'Tekstil', personnel_count: 240,
  total_area_m2: '18500', contact_name: 'Ayşe Kaya', contact_phone: '0224 000 00 00', created_at: '2026-01-01T00:00:00+03:00', updated_at: '2026-01-01T00:00:00+03:00',
};

const meta = {
  title: 'Features/Carbon/CompanyDetails',
  component: CompanyView,
  args: { company, companyName: 'Anadolu Tekstil', canEdit: true },
} satisfies Meta<typeof CompanyView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Editor: Story = {};
export const Restricted: Story = { args: { company: undefined, canEdit: false } };
