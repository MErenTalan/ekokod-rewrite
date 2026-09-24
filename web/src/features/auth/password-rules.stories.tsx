import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { PasswordRules } from './password-rules';

const meta = { title: 'Features/Auth/PasswordRules', component: PasswordRules, args: { password: '' } } satisfies Meta<typeof PasswordRules>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = {};
export const Partial: Story = { args: { password: 'sifre1234' } };
export const Strong: Story = { args: { password: 'Guvenli!Sifre-42' } };
