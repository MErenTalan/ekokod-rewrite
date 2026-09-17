import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { EventDialogView } from './event-dialog';
import { DEFAULT_EVENT_COLOUR } from '@/styles/event-palette';

import { emptyEvent } from './event-draft';

const draft = { ...emptyEvent('2026-03-14', DEFAULT_EVENT_COLOUR), title: 'Planlı bakım' };

const meta = {
  title: 'Features/Calendar/EventDialog',
  component: EventDialogView,
  tags: ['open'],
  args: { open: true, onOpenChange: () => {}, value: draft, onChange: () => {}, onSubmit: () => {} },
} satisfies Meta<typeof EventDialogView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const AllDay: Story = {};
export const Timed: Story = { args: { value: { ...draft, allDay: false } } };
export const Existing: Story = { args: { value: { ...draft, id: 'e-1' }, onDelete: () => {} } };
export const ReadOnly: Story = { args: { value: { ...draft, id: 'e-1' }, readOnly: true } };
