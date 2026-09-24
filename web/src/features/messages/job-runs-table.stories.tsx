import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoRuns } from './_fixture';
import { JobRunsTableView } from './job-runs-table';

const TRIGGERABLE = ['alarm.evaluate', 'billing.dispatch'] as const;

const meta = {
  title: 'Features/Messages/JobRunsTable',
  component: JobRunsTableView,
  args: { runs: demoRuns, triggerable: TRIGGERABLE, canTrigger: true, onTrigger: () => {} },
} satisfies Meta<typeof JobRunsTableView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Admin: Story = {};
/** A company admin reads the history but cannot start a job (R220). */
export const CompanyAdmin: Story = { args: { canTrigger: false } };
export const Triggering: Story = {
  args: { job: { id: 'task-1', label: 'alarm.evaluate', status: 'running' } },
};
export const Empty: Story = { args: { runs: [] } };
