import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { Download, Plus } from 'lucide-react';

import { Button } from '../ui/button';
import { PageHeader } from './page-header';

const meta = {
  title: 'Shell/PageHeader',
  component: PageHeader,
  args: { title: 'Faturalar', description: 'Bina ve dönem bazında hesaplanmış faturalar' },
  parameters: { nextjs: { appDirectory: true, navigation: { pathname: '/bills' } } },
} satisfies Meta<typeof PageHeader>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    actions: (
      <>
        <Button variant="secondary" iconStart={Download}>
          Dışa aktar
        </Button>
        <Button iconStart={Plus}>Fatura hesapla</Button>
      </>
    ),
  },
};
export const CustomBreadcrumb: Story = { args: { title: 'Merkez Bina · Ağustos 2026', breadcrumb: [{ label: 'Faturalar', href: '/bills' }, { label: 'Merkez Bina · Ağustos 2026' }] } };
export const LongTurkishLabel: Story = { args: { title: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri Analizi', description: 'Endüktif ve kapasitif oranların dağıtım şirketi sınırlarıyla karşılaştırılması' } };
