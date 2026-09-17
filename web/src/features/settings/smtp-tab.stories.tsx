import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { SMTPSettings } from '@/lib/api/types';

import { SmtpTabView } from './smtp-tab';

const settings: SMTPSettings = {
  host: 'smtp.ornek.com.tr',
  port: 465,
  secure: true,
  username: 'bildirim@ornek.com.tr',
  from_address: 'bildirim@ornek.com.tr',
  has_password: true,
  updated_at: '2026-09-01T09:00:00+03:00',
};

const meta = {
  title: 'Features/Settings/SmtpTab',
  component: SmtpTabView,
  args: { settings, onSave: () => {}, onTest: () => {} },
} satisfies Meta<typeof SmtpTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Configured: Story = {};
export const NotConfiguredYet: Story = { args: { settings: null } };
