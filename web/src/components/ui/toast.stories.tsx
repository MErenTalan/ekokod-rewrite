import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from './button';
import { useToast } from './toast';

function Example({ title = 'Tarife kaydedildi' }: { title?: string }) {
  const { toast } = useToast();
  return (
    <div className="flex flex-wrap gap-2">
      <Button onClick={() => toast({ tone: 'success', title, description: 'Sanayi OG Çift Terimli' })}>Başarılı</Button>
      <Button variant="danger" onClick={() => toast({ tone: 'danger', title: 'Fatura hesaplanamadı', action: { label: 'Tekrar dene', onClick: () => {} } })}>
        Hata
      </Button>
    </div>
  );
}

const meta = { title: 'UI/Toast', component: Example } satisfies Meta<typeof Example>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Shown: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'Hata' }));
    await expect(await within(canvasElement.ownerDocument.body).findByText('Fatura hesaplanamadı')).toBeInTheDocument();
  },
};
export const LongTurkishLabel: Story = { args: { title: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri kaydedildi' } };
