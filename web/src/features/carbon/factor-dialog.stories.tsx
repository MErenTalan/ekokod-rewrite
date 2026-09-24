import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from '@/components/ui/button';

import { factor } from './_fixture';
import { FactorDialog, type FactorDialogProps } from './factor-dialog';

/** The dialog behind its trigger, as on the screen: the canvas is never empty. */
function Example(props: Partial<FactorDialogProps>) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="secondary" onClick={() => setOpen(true)}>Değiştir</Button>
      {open ? <FactorDialog factor={factor()} saving={false} errors={{}} onSubmit={() => setOpen(false)} {...props} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

const meta = {
  title: 'Features/Carbon/FactorDialog',
  component: FactorDialog,
  args: { factor: factor(), saving: false, errors: {}, onSubmit: () => {}, onClose: () => {} },
} satisfies Meta<typeof FactorDialog>;
export default meta;
type Story = StoryObj<typeof meta>;

const openIt: Story['play'] = async ({ canvasElement }) => {
  await userEvent.click(within(canvasElement).getByRole('button'));
  await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
};

export const OverrideDialog: Story = { tags: ['open'], render: () => <Example />, play: openIt };
export const ServerRefusal: Story = { tags: ['open'], render: () => <Example errors={{ base_factor: 'Değer aralık dışında.' }} />, play: openIt };
