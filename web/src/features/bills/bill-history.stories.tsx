import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { fn } from 'storybook/test';

import { demoAnalyzers, demoBill } from './_fixture';
import { BillHistoryView } from './bill-history';

const meta = {
  title: 'Features/Bills/BillHistory',
  component: BillHistoryView,
  args: {
    bills: [
      demoBill({ id: 'b-oct', scope: 'building', analyzer_id: undefined, period_key: '2026-10', total_cost: '655.01' }),
      demoBill({ id: 'b-sep', period_key: '2026-09', total_cost: '420.16' }),
      demoBill({ id: 'b-aug' }),
    ],
    buildingNames: { 'b-1': 'A1 Fabrika' },
    analyzers: demoAnalyzers,
    onOpenMonth: fn(),
    onDownloadPdf: fn(),
  },
} satisfies Meta<typeof BillHistoryView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { bills: [] } };
/** More bills exist than one page; older months are reached with the month picker. */
export const Truncated: Story = { args: { truncated: true } };
