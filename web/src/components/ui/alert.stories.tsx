import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Alert } from './alert';
import { Button } from './button';

const meta = { title: 'UI/Alert', component: Alert, args: { tone: 'info', title: '' } } satisfies Meta<typeof Alert>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Tones: Story = {
  render: () => (
    <div className="flex max-w-2xl flex-col gap-3">
      <Alert tone="info" title="Veriler her saat güncellenir">
        Son okuma 17 Eyl 2026 09:00.
      </Alert>
      <Alert tone="success" title="Fatura hesaplandı" />
      <Alert tone="warning" title="3 saatlik ölçüm eksik" action={<Button size="sm" variant="secondary">Yeniden dene</Button>}>
        Toplam tüketim tahmini değer içeriyor.
      </Alert>
      <Alert tone="danger" title="Tarife bulunamadı">
        Bu bina için Ağustos 2026 döneminde geçerli bir tarife yok.
      </Alert>
    </div>
  ),
};
export const LongTurkishLabel: Story = { args: { tone: 'warning', title: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri bu dönemde aşıldı' } };
