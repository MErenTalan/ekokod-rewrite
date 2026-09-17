import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from '@/components/ui/button';
import { DEFAULT_EVENT_COLOUR } from '@/styles/event-palette';

import { EventDialogView, type EventDialogViewProps } from './event-dialog';
import { emptyEvent, type EventDraft } from './event-draft';

const base: EventDraft = { ...emptyEvent('2026-03-14', DEFAULT_EVENT_COLOUR), title: 'Planlı bakım' };

// Controlled dialog: the story owns the opener, so the a11y run has something in
// the canvas and Escape has somewhere to return focus to.
function Example({ value = base, ...props }: Partial<EventDialogViewProps>) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<EventDraft | null>(value);
  return (
    <>
      <Button variant="secondary" onClick={() => setOpen(true)}>
        Etkinlik ekle
      </Button>
      <EventDialogView
        onSubmit={() => setOpen(false)}
        {...props}
        value={draft}
        onChange={setDraft}
        open={open}
        onOpenChange={setOpen}
      />
    </>
  );
}

const openIt = async ({ canvasElement }: { canvasElement: HTMLElement }) => {
  await userEvent.click(within(canvasElement).getByRole('button', { name: 'Etkinlik ekle' }));
  await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
};

const meta = {
  title: 'Features/Calendar/EventDialog',
  component: EventDialogView,
  args: { open: false, onOpenChange: () => {}, value: base, onChange: () => {}, onSubmit: () => {} },
} satisfies Meta<typeof EventDialogView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const AllDay: Story = { tags: ['open'], render: () => <Example />, play: openIt };
export const Timed: Story = { tags: ['open'], render: () => <Example value={{ ...base, allDay: false }} />, play: openIt };
export const Existing: Story = {
  tags: ['open'],
  render: () => <Example value={{ ...base, id: 'e-1' }} onDelete={() => {}} />,
  play: openIt,
};
export const ReadOnly: Story = { tags: ['open'], render: () => <Example value={{ ...base, id: 'e-1' }} readOnly />, play: openIt };
