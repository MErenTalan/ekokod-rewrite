import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { wave } from './_story-data';
import { Sparkline } from './sparkline';

const meta = {
  title: 'Charts/Sparkline',
  component: Sparkline,
  args: { label: 'Son 30 gün tüketim', kind: 'consumption', data: Array.from({ length: 30 }, (_, i) => (i === 17 ? null : wave(i, 6000, 1400, 7))) },
} satisfies Meta<typeof Sparkline>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Generation: Story = { args: { label: 'Son 30 gün üretim', kind: 'generation' } };
