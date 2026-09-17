import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useTranslations } from 'next-intl';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from '../ui/button';
import { ThemeCustomizer } from './theme-customizer';

function Example() {
  const t = useTranslations('shell');
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="secondary" onClick={() => setOpen(true)}>
        {t('customizer.open')}
      </Button>
      <ThemeCustomizer open={open} onOpenChange={setOpen} />
    </>
  );
}

const meta = { title: 'Shell/ThemeCustomizer', component: ThemeCustomizer, args: { open: false, onOpenChange: () => {} } } satisfies Meta<typeof ThemeCustomizer>;
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
