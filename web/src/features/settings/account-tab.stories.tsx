import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { Me, Session } from '@/lib/api/types';
import fixture from '@/lib/session/permissions.fixture.json';

import { AccountTabView } from './account-tab';

const profile: Me = {
  id: 'u-1',
  name: 'Ayşe Kaya',
  email: 'ayse@ornek.com.tr',
  phone: '0532 000 00 00',
  role: 'company_admin',
  locale: 'tr',
  permissions: fixture.company_admin as Me['permissions'],
  session_id: 's-1',
  company: { id: 'c-1', name: 'Anadolu Tekstil' },
};

const sessions: Session[] = [
  { id: 's-1', client: 'web', user_agent: 'Chrome · Windows', ip: '203.0.113.7', created_at: '2026-09-10T09:00:00+03:00', last_used_at: '2026-09-17T08:00:00+03:00', expires_at: '2026-10-17T09:00:00+03:00', current: true },
  { id: 's-2', client: 'mobile', user_agent: 'ekokod iOS', ip: '203.0.113.9', created_at: '2026-09-01T09:00:00+03:00', last_used_at: null, expires_at: '2026-10-01T09:00:00+03:00', current: false },
];

const meta = {
  title: 'Features/Settings/AccountTab',
  component: AccountTabView,
  args: { profile, readOnly: false, sessions, onSaveProfile: () => {}, onChangePassword: () => {}, onRevokeSession: () => {}, onLogoutAll: () => {} },
} satisfies Meta<typeof AccountTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Demo: Story = { args: { readOnly: true, profile: { ...profile, role: 'demo' }, sessions: null } };
export const WithErrors: Story = { args: { fieldErrors: { email: 'Geçerli bir e-posta girin' } } };
