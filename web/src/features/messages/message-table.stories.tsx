import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoMessages } from './_fixture';
import { MessageTableView } from './message-table';

const meta = {
  title: 'Features/Messages/MessageTable',
  component: MessageTableView,
  args: { messages: demoMessages, filtered: false },
} satisfies Meta<typeof MessageTableView>;
export default meta;
type Story = StoryObj<typeof meta>;

/** One row per kind, so all three badges and icons are visible at once. */
export const Default: Story = {};
export const Empty: Story = { args: { messages: [] } };
export const EmptyAfterFiltering: Story = { args: { messages: [], filtered: true } };
export const Loading: Story = { args: { loading: true } };
