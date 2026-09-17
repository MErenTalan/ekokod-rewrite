import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { ReactiveStatusCard } from './reactive-status-card';

const meta = {
  title: 'Domain/ReactiveStatusCard',
  component: ReactiveStatusCard,
  args: {
    period: 'Merkez Bina · Ağustos 2026',
    inductive: { ratio: '0.2240', limit: '0.20' },
    capacitive: { ratio: '0.0410', limit: '0.15' },
    penaltyApplied: true,
    advisory: 'Endüktif oran sınırı aştı; kompanzasyon panosundaki kademeleri kontrol edin.',
  },
} satisfies Meta<typeof ReactiveStatusCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const OverLimit: Story = {};
export const WithinLimits: Story = { args: { inductive: { ratio: '0.0812', limit: '0.20' }, penaltyApplied: false, advisory: undefined } };
export const NoData: Story = { args: { inductive: { ratio: null, limit: '0.20' }, capacitive: { ratio: null, limit: '0.15' }, penaltyApplied: null, advisory: undefined } };
