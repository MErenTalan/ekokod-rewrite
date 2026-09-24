import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoYearly } from './_fixture';
import { YearlyReportView } from './yearly-report';

const meta = {
  title: 'Features/Reports/YearlyReport',
  component: YearlyReportView,
  args: { payload: demoYearly },
} satisfies Meta<typeof YearlyReportView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
/** E-1: no plant target means no achievement rate — never capacity × 1500. */
export const NoTarget: Story = { args: { payload: { ...demoYearly, target: undefined, achievement_pct: undefined } } };
/** R262: no grid factor, and the section says why. */
export const NoCarbonFactor: Story = { args: { payload: { ...demoYearly, carbon: undefined, carbon_reason: 'grid_factor_missing' } } };
/** R262: a net exporter keeps its negative net emission. */
export const NetExporter: Story = { args: { payload: { ...demoYearly, carbon: { ...demoYearly.carbon!, net_t: '-3.24' } } } };
