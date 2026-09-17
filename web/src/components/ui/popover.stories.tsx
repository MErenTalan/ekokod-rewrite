import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useTranslations } from 'next-intl';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from './button';
import { Popover } from './popover';

const meta = { title: 'UI/Popover', component: Popover, args: { trigger: <button />, children: null } } satisfies Meta<typeof Popover>;
export default meta;
type Story = StoryObj<typeof meta>;

function Example() {
  const t = useTranslations('common');
  return (
    <Popover trigger={<Button variant="secondary">{t('more')}</Button>} label={t('more')}>
      <div className="flex w-64 flex-col gap-2">
        <p className="text-foreground-muted type-small">Merkez Bina · 12 analizör · son okuma 17 Eyl 2026</p>
        <Button size="sm">{t('apply')}</Button>
      </div>
    </Popover>
  );
}

export const Default: Story = { render: () => <Example /> };

export const Open: Story = {
  tags: ['open'],
  render: () => <Example />,
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button'));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
  },
};
