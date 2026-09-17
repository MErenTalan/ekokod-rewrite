import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Breadcrumb } from './breadcrumb';

const meta = {
  title: 'UI/Breadcrumb',
  component: Breadcrumb,
  args: { items: [{ label: 'Faturalar ve Tarifeler', href: '/bills' }, { label: 'Faturalar', href: '/bills' }, { label: 'Merkez Bina · Ağustos 2026' }] },
} satisfies Meta<typeof Breadcrumb>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const LongTurkishLabel: Story = {
  args: { items: [{ label: 'Veri Analizi', href: '/consumption' }, { label: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri', href: '/x' }, { label: 'İstanbul Soğuk Hava Deposu' }] },
};
