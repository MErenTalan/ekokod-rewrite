import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoAlarms } from './_fixture';
import { AlarmsTabView } from './alarms-tab';

const meta = {
  title: 'Features/SolarPlants/Alarms',
  component: AlarmsTabView,
  args: { items: demoAlarms.items, total: 60, onLoadMore: () => {} },
} satisfies Meta<typeof AlarmsTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { items: [], total: 0, onLoadMore: undefined } };
