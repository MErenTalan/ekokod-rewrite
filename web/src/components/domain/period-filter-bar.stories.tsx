import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';

import type { DateRange } from '../ui/date-range-picker';
import { PeriodFilterBar, type Granularity } from './period-filter-bar';

function Stateful({ applying = false, allowed }: { applying?: boolean; allowed?: Granularity[] }) {
  const [granularity, setGranularity] = useState<Granularity>(allowed?.[0] ?? 'daily');
  const [range, setRange] = useState<DateRange>({ from: '2026-09-01', to: '2026-09-17' });
  return (
    <PeriodFilterBar
      granularity={granularity}
      onGranularityChange={setGranularity}
      range={range}
      onRangeChange={setRange}
      onApply={() => {}}
      today="2026-09-17"
      applying={applying}
      allowedGranularities={allowed}
    />
  );
}

const meta = {
  title: 'Domain/PeriodFilterBar',
  component: PeriodFilterBar,
  args: { granularity: 'daily', onGranularityChange: () => {}, range: { from: '2026-09-01', to: '2026-09-17' }, onRangeChange: () => {}, onApply: () => {}, today: '2026-09-17' },
} satisfies Meta<typeof PeriodFilterBar>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const Applying: Story = { render: () => <Stateful applying /> };
export const MonthlyOnly: Story = { render: () => <Stateful allowed={['monthly', 'yearly']} /> };
