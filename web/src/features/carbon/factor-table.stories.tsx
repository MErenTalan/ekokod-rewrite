import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { fn } from 'storybook/test';

import { factor } from './_fixture';
import { FactorTable } from './factor-table';

const rows = [factor(), factor({ id: 'f-own', key: 'grid_electricity_tr_2022', label: 'Şebeke elektriği', main_category: 'cat_electricity',
  base_factor: '0.44', base_unit: 'kWh', overridden: true, platform_base_factor: '0.469', source: 'Kendi ölçümümüz', source_year: 2026 })];

const meta = {
  title: 'Features/Carbon/FactorTable',
  component: FactorTable,
  args: { rows, editable: true, query: '', onQuery: fn(), main: 'all', onMain: fn(), onOverride: fn(), onReset: fn() },
} satisfies Meta<typeof FactorTable>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Catalogue: Story = {};
export const ReadOnly: Story = { args: { editable: false } };
export const NoMatch: Story = { args: { rows: [] } };
