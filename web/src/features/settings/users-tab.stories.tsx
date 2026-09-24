import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { User } from '@/lib/api/types';

import { UsersTabView } from './users-tab';

const users = [
  { id: 'u-1', name: 'Ayşe Kaya', email: 'ayse@ornek.com.tr', phone: '0532 000 00 00', role: 'company_admin', is_active: true, locale: 'tr', created_at: '', last_login_at: '2026-09-17T08:00:00+03:00' },
  { id: 'u-2', name: 'Can Demir', email: 'can@ornek.com.tr', role: 'company_readonly_admin', is_active: false, locale: 'tr', created_at: '', last_login_at: null },
] as User[];

const meta = {
  title: 'Features/Settings/UsersTab',
  component: UsersTabView,
  args: { users, selfId: 'u-1', canEdit: true, onAdd: () => {}, onEdit: () => {}, onDelete: () => {} },
} satisfies Meta<typeof UsersTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Editable: Story = {};
export const ReadOnly: Story = { args: { canEdit: false } };
export const Empty: Story = { args: { users: [] } };
