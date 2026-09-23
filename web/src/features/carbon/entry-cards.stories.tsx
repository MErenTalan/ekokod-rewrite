import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { fn } from 'storybook/test';

import { catalogue } from './_fixture';
import { EntryCards } from './entry-cards';

const subs = catalogue.items.flatMap((m) => m.subs.map((s) => ({ ...s, main: m.key })));

const meta = {
  title: 'Features/Carbon/EntryCards',
  component: EntryCards,
  args: { subs, counts: { sub_space_heating: 3, sub_grid_electricity: 30 }, full: false, editable: true, onAdd: fn(), onGoSelection: fn() },
} satisfies Meta<typeof EntryCards>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Cards: Story = {};
export const NothingSelected: Story = { args: { subs: [] } };
export const ReadOnly: Story = { args: { editable: false } };
