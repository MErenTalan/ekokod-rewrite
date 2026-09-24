import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from '@/components/ui/button';
import type { ConsumptionRow } from '@/lib/api/types';

import { AlarmCheckDialogView, type AlarmCheckDialogViewProps } from './alarm-check-dialog';

const row = { period_start: '2026-03-14T00:00:00+03:00', active_import: '1234.5' } as ConsumptionRow;

// The dialog is controlled, so the story owns the opener: the a11y run needs
// something in the canvas and Escape has to have somewhere to return focus to.
function Example(props: Partial<AlarmCheckDialogViewProps>) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="secondary" onClick={() => setOpen(true)}>
        Alarm kontrolü
      </Button>
      <AlarmCheckDialogView
        row={row}
        granularity="daily"
        result={{ available: false, reason: 'ml_service_unavailable' }}
        {...props}
        open={open}
        onOpenChange={setOpen}
      />
    </>
  );
}

const openIt = async ({ canvasElement }: { canvasElement: HTMLElement }) => {
  await userEvent.click(within(canvasElement).getByRole('button', { name: 'Alarm kontrolü' }));
  await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
};

const meta = {
  title: 'Features/Consumption/AlarmCheckDialog',
  component: AlarmCheckDialogView,
  args: { open: false, onOpenChange: () => {}, row, granularity: 'daily' as const, result: null },
} satisfies Meta<typeof AlarmCheckDialogView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Unavailable: Story = { tags: ['open'], render: () => <Example />, play: openIt };
export const Checking: Story = { tags: ['open'], render: () => <Example result={null} loading />, play: openIt };
