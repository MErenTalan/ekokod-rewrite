import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { fn } from 'storybook/test';

import { note } from './_fixture';
import { NotesView } from './notes-view';

const meta = {
  title: 'Features/Iso50001/Notes',
  component: NotesView,
  args: { notes: [note(), note({ id: 'n-2', title: undefined, body: 'Toplantı tutanağı eklendi.' })], editable: true, saving: false, onAdd: fn(), onUpdate: fn(), onDelete: fn() },
} satisfies Meta<typeof NotesView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Notes: Story = {};
export const Empty: Story = { args: { notes: [] } };
export const ReadOnly: Story = { args: { editable: false } };
