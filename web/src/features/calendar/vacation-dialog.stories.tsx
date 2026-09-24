import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from '@/components/ui/button';

import { demoPeriods, demoWeekendDays } from './_fixture';
import { VacationDialogView, type VacationDialogViewProps } from './vacation-dialog';

// Controlled dialog: the story owns the opener, so the a11y run has something in
// the canvas and Escape has somewhere to return focus to.
function Example(props: Partial<VacationDialogViewProps>) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="secondary" onClick={() => setOpen(true)}>
        Tatil yönetimi
      </Button>
      <VacationDialogView
        weekendDays={demoWeekendDays}
        periods={demoPeriods}
        onSave={() => setOpen(false)}
        {...props}
        open={open}
        onOpenChange={setOpen}
      />
    </>
  );
}

const openIt = async ({ canvasElement }: { canvasElement: HTMLElement }) => {
  await userEvent.click(within(canvasElement).getByRole('button', { name: 'Tatil yönetimi' }));
  await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
};

const meta = {
  title: 'Features/Calendar/VacationDialog',
  component: VacationDialogView,
  args: { open: false, onOpenChange: () => {}, weekendDays: demoWeekendDays, periods: demoPeriods, onSave: () => {} },
} satisfies Meta<typeof VacationDialogView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { tags: ['open'], render: () => <Example />, play: openIt };
export const EveryDayNonWorking: Story = {
  tags: ['open'],
  render: () => <Example weekendDays={[0, 1, 2, 3, 4, 5, 6]} />,
  play: openIt,
};
export const ReadOnly: Story = { tags: ['open'], render: () => <Example readOnly />, play: openIt };
