import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoEvents, demoPeriods, demoWeekendDays } from './_fixture';
import { TimeGridView } from './time-grid-view';

const meta = {
  title: 'Features/Calendar/TimeGridView',
  component: TimeGridView,
  args: {
    anchor: '2026-03-10',
    events: demoEvents,
    weekendDays: demoWeekendDays,
    periods: demoPeriods,
    canEdit: true,
    onSelectDate: () => {},
    onSelectEvent: () => {},
    view: 'week' as const,
  },
} satisfies Meta<typeof TimeGridView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const ReadOnly: Story = { args: { canEdit: false } };
export const DayView: Story = { args: { view: 'day' } };
export const NoEvents: Story = { args: { events: [] } };
