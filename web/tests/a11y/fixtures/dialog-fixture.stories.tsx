import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { Dialog } from 'radix-ui';
import { expect, userEvent, within } from 'storybook/test';

// Harness self-test (plan T2 Step 5): a bare Radix modal proves keyboard.spec's overlay logic without T4's Dialog.
const meta = { title: 'Fixtures/DialogFixture' } satisfies Meta;
export default meta;

export const Open: StoryObj = {
  tags: ['open'],
  render: () => (
    <Dialog.Root>
      <Dialog.Trigger className="rounded-md border border-border-control bg-surface px-3 py-2 text-foreground pointer-coarse:min-h-11">
        Aç
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-overlay" />
        <Dialog.Content className="fixed start-1/2 top-1/2 flex w-80 -translate-x-1/2 -translate-y-1/2 flex-col gap-3 rounded-lg bg-surface-raised p-4 text-foreground">
          <Dialog.Title className="type-h3">Fatura notu</Dialog.Title>
          <Dialog.Description className="text-foreground-muted">Notu kaydetmeden kapatabilirsiniz.</Dialog.Description>
          <input aria-label="Not" className="h-9 rounded-md border border-border-control bg-surface px-2 pointer-coarse:min-h-11" />
          <Dialog.Close className="rounded-md bg-primary px-3 py-2 text-on-primary pointer-coarse:min-h-11">Kapat</Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  ),
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'Aç' }));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
  },
};
