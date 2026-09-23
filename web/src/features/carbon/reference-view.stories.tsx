import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { catalogue } from './_fixture';
import { ReferenceView } from './reference-view';

const meta = {
  title: 'Features/Carbon/Reference',
  component: ReferenceView,
  args: { kind: 'ghg', catalogue },
} satisfies Meta<typeof ReferenceView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const GhgProtocol: Story = {};
export const Iso14064: Story = { args: { kind: 'iso' } };
export const Standards: Story = { args: { kind: 'standards' } };
