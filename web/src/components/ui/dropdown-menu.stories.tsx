import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { Download, FileSpreadsheet, Mail, Trash2 } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from './button';
import { DropdownMenu } from './dropdown-menu';

function Example({ longLabel = false }: { longLabel?: boolean }) {
  const t = useTranslations('common');
  return (
    <DropdownMenu
      align="start"
      trigger={<Button variant="secondary">{t('actions')}</Button>}
      items={[
        { type: 'label', label: 'Dışa aktar' },
        { type: 'item', label: 'CSV', icon: Download, onSelect: () => {} },
        { type: 'item', label: longLabel ? 'Reaktif Endüktif Tüketim Oranı Eşik Değeri raporu' : 'Excel', icon: FileSpreadsheet, onSelect: () => {} },
        { type: 'item', label: 'E-posta ile gönder', icon: Mail, description: 'SMTP ayarları eksik', disabled: true, onSelect: () => {} },
        { type: 'separator' },
        { type: 'item', label: t('delete'), icon: Trash2, tone: 'danger', onSelect: () => {} },
      ]}
    />
  );
}

const meta = { title: 'UI/DropdownMenu', component: DropdownMenu, args: { trigger: <button />, items: [] } } satisfies Meta<typeof DropdownMenu>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Example /> };
export const Open: Story = {
  tags: ['open', 'modal-popup'],
  render: () => <Example />,
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button'));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('menu')).toBeInTheDocument();
  },
};
export const LongTurkishLabel: Story = { render: () => <Example longLabel /> };
