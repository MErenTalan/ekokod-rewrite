import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoEvents, demoPeriods, demoWeekendDays } from './_fixture';
import { MonthView } from './month-view';

const meta = {
  title: 'Features/Calendar/MonthView',
  component: MonthView,
  args: {
    anchor: '2026-03-14',
    events: demoEvents,
    weekendDays: demoWeekendDays,
    periods: demoPeriods,
    canEdit: true,
    onSelectDate: () => {},
    onSelectEvent: () => {},
  },
} satisfies Meta<typeof MonthView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const ReadOnly: Story = { args: { canEdit: false } };
export const NoEvents: Story = { args: { events: [] } };
