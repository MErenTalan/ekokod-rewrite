import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from '@/components/ui/button';

import { EmailDialog, type EmailDialogProps } from './email-dialog';

/** The dialog behind its trigger, as on the screen: the canvas is never empty. */
function Example(props: Partial<EmailDialogProps>) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="secondary" onClick={() => setOpen(true)}>E-posta ile gönder</Button>
      <EmailDialog
        summary={{ period: 'Ağustos 2026', building: 'Merkez' }}
        sending={false}
        onSend={() => setOpen(false)}
        {...props}
        open={open}
        onClose={() => setOpen(false)}
      />
    </>
  );
}

const meta = {
  title: 'Features/Reports/EmailDialog',
  component: EmailDialog,
  args: { open: false, summary: { period: 'Ağustos 2026', building: 'Merkez' }, sending: false, onSend: () => {}, onClose: () => {} },
} satisfies Meta<typeof EmailDialog>;
export default meta;
type Story = StoryObj<typeof meta>;

const openIt: Story['play'] = async ({ canvasElement }) => {
  await userEvent.click(within(canvasElement).getByRole('button'));
  await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
};

export const Default: Story = { tags: ['open'], render: () => <Example />, play: openIt };
/** The server's refusal of the address is shown on the field (R266). */
export const ServerRefusal: Story = { tags: ['open'], render: () => <Example error="Geçerli bir e-posta adresi girin." />, play: openIt };
