import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { fn } from 'storybook/test';

import { emptyOverview, overview } from './_fixture';
import { OverviewView } from './overview-view';

const meta = {
  title: 'Features/Carbon/Overview',
  component: OverviewView,
  args: { overview, years: [2026, 2025, 2024], onYearChange: fn(), onSeeAll: fn() },
} satisfies Meta<typeof OverviewView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Year: Story = {};
export const Empty: Story = { args: { overview: emptyOverview } };
export const Loading: Story = { args: { loading: true } };
