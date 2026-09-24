import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { AuthSplitLayout } from './auth-split-layout';

const meta = {
  title: 'Features/Auth/AuthSplitLayout',
  component: AuthSplitLayout,
  parameters: { layout: 'fullscreen' },
  args: { children: <h1 className="text-foreground type-h1">Kullanıcı Girişi</h1> },
} satisfies Meta<typeof AuthSplitLayout>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
