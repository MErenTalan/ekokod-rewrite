import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoProfiles } from './_fixture';
import { DailyTabView } from './daily-tab';

const meta = {
  title: 'Features/LoadProfile/DailyTab',
  component: DailyTabView,
  args: { data: demoProfiles },
} satisfies Meta<typeof DailyTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { data: null } };
export const Loading: Story = { args: { loading: true } };
