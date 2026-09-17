import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useTranslations } from 'next-intl';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from './button';
import { Dialog, type DialogProps } from './dialog';
import { Input } from './input';

function Example(props: Partial<DialogProps>) {
  const t = useTranslations('common');
  const [open, setOpen] = useState(false);
  return (
    <Dialog
      title="Tarifeyi düzenle"
      description="Değişiklikler sonraki fatura döneminden itibaren geçerli olur."
      trigger={<Button variant="secondary">{t('edit')}</Button>}
      footer={
        <>
          <Button variant="ghost" onClick={() => setOpen(false)}>
            {t('cancel')}
          </Button>
          <Button onClick={() => setOpen(false)}>{t('save')}</Button>
        </>
      }
      {...props}
      open={open}
      onOpenChange={setOpen}
    >
      <Input label="Tarife adı" defaultValue="Sanayi OG Çift Terimli" />
    </Dialog>
  );
}

const meta = { title: 'UI/Dialog', component: Dialog, args: { open: false, onOpenChange: () => {}, title: '', children: null } } satisfies Meta<typeof Dialog>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Example /> };
export const Open: Story = {
  tags: ['open'],
  render: () => <Example />,
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button'));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
  },
};
export const LongTurkishLabel: Story = { render: () => <Example title="Reaktif Endüktif Tüketim Oranı Eşik Değeri güncellemesi" size="sm" /> };
