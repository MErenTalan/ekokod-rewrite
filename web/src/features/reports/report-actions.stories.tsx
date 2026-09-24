import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { ReportActionsView } from './report-actions';

const meta = {
  title: 'Features/Reports/ReportActions',
  component: ReportActionsView,
  args: {
    canGenerate: true, canEmail: true, generating: false,
    rows: [
      { buildingId: 'b-1', buildingName: 'Merkez', reportId: 'r-1', status: 'succeeded' },
      { buildingId: 'b-2', buildingName: 'Depo', reportId: 'r-2', status: 'running' },
      { buildingId: 'b-3', buildingName: 'Atölye', reportId: 'r-3', status: 'failed', errorCode: 'report_building_not_found' },
    ],
    onGenerate: () => {}, onDownload: () => {}, onEmail: () => {},
  },
} satisfies Meta<typeof ReportActionsView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const PerBuilding: Story = {};
export const BeforeGenerating: Story = { args: { rows: [] } };
/** A read-only admin: downloads of ready reports only. */
export const ReadOnly: Story = { args: { canGenerate: false, canEmail: false } };
