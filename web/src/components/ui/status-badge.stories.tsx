import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { StatusBadge } from './status-badge';

const meta = { title: 'UI/StatusBadge', component: StatusBadge, args: { status: 'success', label: 'Sınır içinde' } } satisfies Meta<typeof StatusBadge>;
export default meta;
type Story = StoryObj<typeof meta>;

export const All: Story = {
  render: () => (
    <div className="flex flex-wrap gap-2">
      <StatusBadge status="success" label="Sınır içinde" />
      <StatusBadge status="warning" label="Eksik veri" />
      <StatusBadge status="danger" label="Sınır aşıldı" />
      <StatusBadge status="info" label="Hesaplanıyor" />
      <StatusBadge status="neutral" label="Veri yok" />
    </div>
  ),
};
export const LongTurkishLabel: Story = { args: { status: 'danger', label: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri aşıldı' } };
