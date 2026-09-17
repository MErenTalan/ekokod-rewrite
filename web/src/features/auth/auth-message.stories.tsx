import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { AuthMessage } from './auth-message';

const meta = {
  title: 'Features/Auth/AuthMessage',
  component: AuthMessage,
  args: {
    tone: 'danger',
    title: 'Giriş yapılamadı',
    description: 'Oturumunuz açılırken bir sorun oluştu. Lütfen tekrar giriş yapmayı deneyin.',
    action: { href: '/auth/login', label: 'Girişe Dön' },
  },
} satisfies Meta<typeof AuthMessage>;
export default meta;
type Story = StoryObj<typeof meta>;

export const AuthError: Story = {};
export const Maintenance: Story = {
  args: {
    tone: 'warning',
    title: 'Bakım çalışması',
    description: 'EKORM kısa bir bakım çalışması nedeniyle geçici olarak kullanılamıyor. Lütfen biraz sonra tekrar deneyin.',
    action: { href: '/ekorm', label: 'Tekrar Dene' },
  },
};
