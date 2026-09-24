import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { fn } from 'storybook/test';

import { catalogue } from './_fixture';
import { SelectionView } from './selection-view';

const meta = {
  title: 'Features/Carbon/Selection',
  component: SelectionView,
  args: { catalogue, selected: ['sub_space_heating', 'sub_waste_disposal'], editable: true, onSave: fn() },
} satisfies Meta<typeof SelectionView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Editable: Story = {};
export const ReadOnly: Story = { args: { editable: false } };
