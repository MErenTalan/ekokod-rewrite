import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { monthly } from './_fixture';
import { OffsetCard } from './offset-card';

const meta = {
  title: 'Features/Financial/OffsetCard',
  component: OffsetCard,
  args: { figures: monthly.total },
} satisfies Meta<typeof OffsetCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Computed: Story = {};
export const Unavailable: Story = { args: { figures: monthly.items[2] } };
