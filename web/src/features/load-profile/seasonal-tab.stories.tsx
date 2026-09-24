import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoProfiles } from './_fixture';
import { SeasonalTabView } from './seasonal-tab';

const meta = {
  title: 'Features/LoadProfile/SeasonalTab',
  component: SeasonalTabView,
  args: { data: demoProfiles },
} satisfies Meta<typeof SeasonalTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { data: null } };
export const Loading: Story = { args: { loading: true } };
