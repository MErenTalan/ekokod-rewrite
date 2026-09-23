import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { overview } from './_fixture';
import { SummaryCardsView } from './summary-cards';

const meta = {
  title: 'Features/Renewable/SummaryCards',
  component: SummaryCardsView,
  args: { data: overview },
} satisfies Meta<typeof SummaryCardsView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Totals: Story = {};
export const NoGeneration: Story = {
  args: { data: { unavailable: { active_generation_kwh: 'no_data' } } },
};
