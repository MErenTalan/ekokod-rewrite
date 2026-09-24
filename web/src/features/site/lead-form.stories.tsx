import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { LeadForm } from './lead-form';

const meta = {
  title: 'Features/Site/LeadForm',
  component: LeadForm,
  args: { kind: 'contact', errors: {}, status: 'idle', sending: false, onSubmit: () => {}, onReset: () => {} },
} satisfies Meta<typeof LeadForm>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Contact: Story = {};
export const Demo: Story = { args: { kind: 'demo' } };
export const WithErrors: Story = { args: { errors: { email: 'Geçerli bir e-posta girin', message: 'En az 10 karakter yazın.' } } };
export const Sending: Story = { args: { sending: true } };
export const NotConfigured: Story = { args: { status: 'notConfigured' } };
export const Sent: Story = { args: { status: 'sent' } };
