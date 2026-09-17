import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Select } from '../ui/select';
import { FilterBar } from './filter-bar';

const Filters = () => (
  <div className="w-56">
    <Select label="Çözünürlük" options={[{ value: 'hourly', label: 'Saatlik' }, { value: 'daily', label: 'Günlük' }]} value="hourly" onValueChange={() => {}} />
  </div>
);

const meta = { title: 'Shell/FilterBar', component: FilterBar, args: { children: <Filters />, activeCount: 1, onApply: () => {} } } satisfies Meta<typeof FilterBar>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Mobile: Story = { parameters: { viewport: { value: 'mobile1' } } };
