import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { RefreshActionsView } from './refresh-actions';

const meta = {
  title: 'Features/Jobs/RefreshActions',
  component: RefreshActionsView,
  args: { analyzerId: 'a-1', pending: false, job: null, onRefresh: () => {}, onDismiss: () => {} },
} satisfies Meta<typeof RefreshActionsView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const NoAnalyzer: Story = { args: { analyzerId: null } };
export const Running: Story = {
  args: { job: { id: 'job-7', label: 'Saatlik değerleri yenile', status: 'running' } },
};
export const Succeeded: Story = {
  args: {
    job: {
      id: 'job-7',
      label: 'Saatlik değerleri yenile',
      status: 'succeeded',
      message: 'Veri çekme başlatıldı; değerler birkaç dakika içinde görünür.',
    },
  },
};
