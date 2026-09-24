import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { AuthSplitLayout } from './auth-split-layout';
import { LoginForm } from './login-form';

const meta = { title: 'Features/Auth/LoginForm', component: LoginForm, args: {} } satisfies Meta<typeof LoginForm>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const DeviceMismatch: Story = { args: { reason: 'device_mismatch' } };
export const PasswordReset: Story = { args: { reason: 'password_reset' } };
export const InSplitLayout: Story = {
  parameters: { layout: 'fullscreen' },
  render: (args) => (
    <AuthSplitLayout>
      <LoginForm {...args} />
    </AuthSplitLayout>
  ),
};
