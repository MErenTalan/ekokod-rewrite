import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useTranslations } from 'next-intl';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from './button';
import { Drawer, type DrawerProps } from './drawer';
import { Switch } from './switch';

function Example({ side = 'end', title = 'Görünüm ayarları' }: Partial<DrawerProps>) {
  const t = useTranslations('common');
  const [open, setOpen] = useState(false);
  const [compact, setCompact] = useState(true);
  return (
    <Drawer side={side} title={title} open={open} onOpenChange={setOpen} trigger={<Button variant="secondary">{t('more')}</Button>}>
      <Switch label="Kenar çubuğu daraltılmış" checked={compact} onCheckedChange={setCompact} />
    </Drawer>
  );
}

const meta = { title: 'UI/Drawer', component: Drawer, args: { open: false, onOpenChange: () => {}, title: '', children: null, side: 'end' } } satisfies Meta<typeof Drawer>;
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
export const BottomSheet: Story = {
  tags: ['open'],
  render: () => <Example side="bottom" title="Filtreler" />,
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button'));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
  },
};
export const LongTurkishLabel: Story = { render: () => <Example side="start" title="Reaktif Endüktif Tüketim Oranı Eşik Değeri ayarları" /> };
