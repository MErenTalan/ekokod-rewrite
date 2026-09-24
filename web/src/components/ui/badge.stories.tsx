import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Badge } from './badge';

const meta = { title: 'UI/Badge', component: Badge, args: { tone: 'neutral', children: 'Pasif' } } satisfies Meta<typeof Badge>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Tones: Story = {
  render: () => (
    <div className="flex flex-wrap gap-2">
      <Badge tone="neutral">Taslak</Badge>
      <Badge tone="brand">Yeni</Badge>
      <Badge tone="info">Bilgi</Badge>
      <Badge tone="success">Aktif</Badge>
      <Badge tone="warning">Tahmini</Badge>
      <Badge tone="danger">Ceza</Badge>
    </div>
  ),
};
export const LongTurkishLabel: Story = { args: { tone: 'warning', children: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri' } };
