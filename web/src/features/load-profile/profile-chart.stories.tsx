import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { ProfileChart } from './profile-chart';
import { demoProfiles } from './_fixture';

const meta = {
  title: 'Features/LoadProfile/ProfileChart',
  component: ProfileChart,
  args: { data: demoProfiles, keys: ['weekday'] },
} satisfies Meta<typeof ProfileChart>;
export default meta;
type Story = StoryObj<typeof meta>;

export const SingleProfile: Story = {};
export const Overlaid: Story = { args: { keys: ['weekday', 'weekend', 'summer_weekday'], title: 'Profil karşılaştırması' } };
export const Empty: Story = { args: { data: null } };
