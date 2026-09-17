import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { CompareTabView } from './compare-tab';
import { demoProfiles } from './_fixture';

const meta = {
  title: 'Features/LoadProfile/CompareTab',
  component: CompareTabView,
  args: { data: demoProfiles, selected: ['weekday', 'weekend'], onSelectedChange: () => {} },
} satisfies Meta<typeof CompareTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const TwoProfiles: Story = {};
export const SixProfiles: Story = {
  args: { selected: ['weekday', 'weekend', 'winter_weekday', 'winter_weekend', 'summer_weekday', 'summer_weekend'] },
};
