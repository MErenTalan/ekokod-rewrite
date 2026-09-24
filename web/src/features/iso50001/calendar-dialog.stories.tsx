import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from '@/components/ui/button';

import { clauses, emptyProject, project } from './_fixture';
import { CalendarDialog, type CalendarDialogProps } from './calendar-dialog';

/** The dialog behind its trigger, as on the screen: the canvas is never empty. */
function Example(props: Partial<CalendarDialogProps>) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="secondary" onClick={() => setOpen(true)}>Takvimi Güncelle</Button>
      <CalendarDialog project={project} clauses={clauses} saving={false} onSave={() => setOpen(false)} {...props} open={open} onClose={() => setOpen(false)} />
    </>
  );
}

const meta = {
  title: 'Features/Iso50001/CalendarDialog',
  component: CalendarDialog,
  args: { open: false, project, clauses, saving: false, onSave: () => {}, onClose: () => {} },
} satisfies Meta<typeof CalendarDialog>;
export default meta;
type Story = StoryObj<typeof meta>;

const openIt: Story['play'] = async ({ canvasElement }) => {
  await userEvent.click(within(canvasElement).getByRole('button'));
  await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
};

export const Planned: Story = { tags: ['open'], render: () => <Example />, play: openIt };
export const Empty: Story = { tags: ['open'], render: () => <Example project={emptyProject} />, play: openIt };
