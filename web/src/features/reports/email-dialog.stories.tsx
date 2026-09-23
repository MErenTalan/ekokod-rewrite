import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { EmailDialog } from './email-dialog';

const meta = {
  title: 'Features/Reports/EmailDialog',
  component: EmailDialog,
  args: { open: true, summary: { period: 'Ağustos 2026', building: 'Merkez' }, sending: false, onSend: () => {}, onClose: () => {} },
} satisfies Meta<typeof EmailDialog>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const ServerRefusal: Story = { args: { error: 'Geçerli bir e-posta adresi girin.' } };
