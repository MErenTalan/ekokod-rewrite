import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { DataQualityBadge } from './data-quality-badge';

const meta = { title: 'Domain/DataQualityBadge', component: DataQualityBadge, args: { quality: { state: 'estimated', reason: '6 saatlik ölçüm eksik; geçen haftanın profiliyle tahmin edildi', coverage: '96.4' } } } satisfies Meta<typeof DataQualityBadge>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Estimated: Story = {};
export const Incomplete: Story = { args: { quality: { state: 'incomplete', reason: 'Ağustos 12–14 arası veri yok', coverage: '90.3' } } };
export const Suspect: Story = { args: { quality: { state: 'suspect', reason: 'Sayaç çarpanı değişti; değerler doğrulanmadı' } } };
// Complete data renders no badge; the figure stands alone.
export const Complete: Story = {
  args: { quality: { state: 'complete' } },
  render: (args) => (
    <p className="flex items-center gap-2 text-foreground">
      <span className="type-data">182.345,1 kWh</span>
      <DataQualityBadge {...args} />
    </p>
  ),
};
export const LongTurkishLabel: Story = { args: { quality: { state: 'incomplete', reason: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri hesaplanırken 3 günlük endüktif ölçüm bulunamadı' } } };
