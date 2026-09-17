import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Sidebar } from './sidebar';

const meta = {
  title: 'Shell/Sidebar',
  component: Sidebar,
  args: { collapsed: false },
  parameters: { nextjs: { appDirectory: true, navigation: { pathname: '/load-profile' } } },
} satisfies Meta<typeof Sidebar>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Collapsed: Story = { args: { collapsed: true } };
export const BillsActive: Story = { parameters: { nextjs: { appDirectory: true, navigation: { pathname: '/bills/2026-08' } } } };
