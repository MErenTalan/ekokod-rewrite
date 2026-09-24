import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { expect, userEvent, within } from 'storybook/test';

import { SiteHeader } from './site-header';

const meta = {
  title: 'Features/Site/SiteHeader',
  component: SiteHeader,
  args: { pricing: false, signedIn: false },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof SiteHeader>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Visitor: Story = {};
export const SignedInWithPricing: Story = { args: { signedIn: true, pricing: true } };
export const MobileDrawer: Story = {
  tags: ['open'],
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'Menüyü aç' }));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
  },
};
