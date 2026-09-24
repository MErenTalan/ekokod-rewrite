import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoAlarms } from './_fixture';
import { AlarmTableView } from './alarm-table';

const meta = {
  title: 'Features/Alarms/AlarmTable',
  component: AlarmTableView,
  args: {
    alarms: demoAlarms,
    state: 'all' as const,
    onStateChange: () => {},
    canEdit: true,
    canEvaluate: true,
    onCreate: () => {},
    onEdit: () => {},
    onDelete: () => {},
    onToggle: () => {},
    onDetails: () => {},
    onEvaluate: () => {},
  },
} satisfies Meta<typeof AlarmTableView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
/** A building admin may edit but not start a dry run (alarms.evaluate is A/CA). */
export const WithoutEvaluate: Story = { args: { canEvaluate: false } };
export const ReadOnly: Story = { args: { canEdit: false, canEvaluate: false } };
export const Empty: Story = { args: { alarms: [] } };
export const Loading: Story = { args: { loading: true } };
