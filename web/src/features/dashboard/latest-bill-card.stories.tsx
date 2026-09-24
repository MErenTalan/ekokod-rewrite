import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { Bill } from '@/lib/api/types';

import { LatestBillCardView } from './latest-bill-card';

const bill: Bill = {
  id: 'bill-1',
  company_id: 'c-1',
  scope: 'building',
  status: 'issued',
  period_key: '2026-08',
  period_start: '2026-08-01',
  period_end: '2026-09-01',
  days_in_period: 31,
  currency: 'TRY',
  active_import: '18450.500',
  net_consumption: '18450.500',
  inductive_kvarh: '3321.090',
  capacitive_kvarh: '553.500',
  energy_cost: '31800.25',
  distribution_cost: '8400.10',
  power_cost: '2700.00',
  reactive_penalty: '0',
  reactive_penalty_applied: false,
  vat_cost: '5350.40',
  total_cost: '48250.75',
  computed_at: '2026-09-01T03:00:00+03:00',
  has_pdf: true,
  ptf_yekdem_used: false,
} as Bill;

const meta = {
  title: 'Features/Dashboard/LatestBillCard',
  component: LatestBillCardView,
  args: { bill, scopeLabel: 'A1 Fabrika' },
} satisfies Meta<typeof LatestBillCardView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Flagged: Story = { args: { bill: { ...bill, status: 'flagged' } } };
export const Empty: Story = { args: { bill: null, scopeLabel: 'Şirket geneli' } };
export const Loading: Story = { args: { loading: true } };
