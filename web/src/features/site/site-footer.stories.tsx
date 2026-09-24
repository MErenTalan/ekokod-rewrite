import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { SiteFooter } from './site-footer';

const meta = {
  title: 'Features/Site/SiteFooter',
  component: SiteFooter,
  args: { year: 2026, pricing: false },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof SiteFooter>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const WithPricing: Story = { args: { pricing: true } };
