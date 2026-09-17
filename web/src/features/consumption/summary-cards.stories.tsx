import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { ConsumptionSummary } from '@/lib/api/types';

import { SummaryCardsView } from './summary-cards';

const summary = {
  rows: 31,
  suspect_rows: 0,
  totals: { active_import: '18450.5', reactive_inductive_import: '3321.09', reactive_capacitive_import: '553.5' },
  averages: { active_import: '595.18' },
} as ConsumptionSummary;

const meta = {
  title: 'Features/Consumption/SummaryCards',
  component: SummaryCardsView,
  args: { summary },
} satisfies Meta<typeof SummaryCardsView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Suspect: Story = { args: { summary: { ...summary, suspect_rows: 3 } } };
export const Loading: Story = { args: { loading: true } };
