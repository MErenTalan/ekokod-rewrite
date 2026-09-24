import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { Company } from '@/lib/api/types';

import { CompanyListView } from './company-list';

const companies: Company[] = [
  { id: 'c-own', name: 'Ekokod Platform', sector: 'Hizmet', created_at: '', updated_at: '' },
  { id: 'c-a', name: 'Anadolu Tekstil A.Ş.', sector: 'Üretim', created_at: '', updated_at: '' },
  { id: 'c-b', name: 'Boğaziçi Gıda Ltd. Şti.', sector: 'Gıda', created_at: '', updated_at: '' },
];

const meta = {
  title: 'Features/Settings/CompanyList',
  component: CompanyListView,
  args: {
    companies,
    ownCompanyId: 'c-own',
    hasMore: true,
    onLoadMore: () => {},
    onActAs: () => {},
    onCreate: () => {},
    onDelete: () => {},
  },
} satisfies Meta<typeof CompanyListView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const ActingForAnother: Story = { args: { activeCompanyId: 'c-b' } };
export const Empty: Story = { args: { companies: [], hasMore: false } };
