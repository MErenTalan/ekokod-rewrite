import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { UserFormView, emptyUser } from './user-form';

const meta = {
  title: 'Features/Settings/UserForm',
  component: UserFormView,
  args: { value: emptyUser('company_admin'), onChange: () => {}, actorRole: 'company_admin' as const },
} satisfies Meta<typeof UserFormView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const NewUser: Story = {};
export const AdminCreating: Story = { args: { value: emptyUser('admin'), actorRole: 'admin' } };
export const OwnAccount: Story = {
  args: { value: { ...emptyUser('admin'), id: 'u-1', name: 'Ben', email: 'ben@ornek.com.tr' }, actorRole: 'admin', isSelf: true },
};
