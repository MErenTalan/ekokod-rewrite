import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { ResetPasswordForm } from './reset-password-form';

const meta = { title: 'Features/Auth/ResetPasswordForm', component: ResetPasswordForm, args: { token: 'story-token' } } satisfies Meta<
  typeof ResetPasswordForm
>;
export default meta;
type Story = StoryObj<typeof meta>;

export const WithToken: Story = {};
export const MissingToken: Story = { args: { token: undefined } };
