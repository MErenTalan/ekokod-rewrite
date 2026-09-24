import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { storyUser } from './_story-user';
import { TopBar } from './top-bar';

const meta = {
  title: 'Shell/TopBar',
  component: TopBar,
  args: { user: storyUser, notificationCount: 12, sidebarOpen: false, onToggleSidebar: () => {}, onOpenCustomizer: () => {} },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof TopBar>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const NoNotifications: Story = { args: { notificationCount: 0 } };
