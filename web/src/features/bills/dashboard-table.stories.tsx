import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoDashboard, demoDivergentDashboard } from './_fixture';
import { DashboardTableView } from './dashboard-table';

const meta = {
  title: 'Features/Bills/DashboardTable',
  component: DashboardTableView,
  args: {
    buildings: demoDashboard.buildings,
    onDownloadPdf: () => {},
    onDownloadHourly: () => {},
    onDownloadAll: () => {},
    onDownloadAllPdf: () => {},
  },
} satisfies Meta<typeof DashboardTableView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
/** 02 §6.11 prices a building invoice over the aggregate, so it may differ (R234). */
export const WithDivergentBuildingInvoice: Story = { args: { buildings: demoDivergentDashboard.buildings } };
export const Empty: Story = { args: { buildings: [] } };
export const Loading: Story = { args: { loading: true } };
