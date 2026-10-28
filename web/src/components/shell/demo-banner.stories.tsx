import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { DemoBanner } from './demo-banner';

const meta = {
  title: 'Shell/DemoBanner',
  component: DemoBanner,
  args: { resetsAt: '2026-10-28T12:00:00Z' },
} satisfies Meta<typeof DemoBanner>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const English: Story = { globals: { locale: 'en' } };
