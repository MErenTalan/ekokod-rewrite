import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { fn } from 'storybook/test';

import { report } from './_fixture';
import { ReportHistory } from './report-history';

const meta = {
  title: 'Features/Carbon/ReportHistory',
  component: ReportHistory,
  args: { rows: [report(), report({ id: 'r-2', report_type: 'iso', name: 'ISO 14064 2025' , period: '2025-01-01/2025-12-31' })], hasMore: false, onLoadMore: fn(), onDownload: fn() },
} satisfies Meta<typeof ReportHistory>;
export default meta;
type Story = StoryObj<typeof meta>;

export const History: Story = {};
export const EmptyHistory: Story = { args: { rows: [] } };
