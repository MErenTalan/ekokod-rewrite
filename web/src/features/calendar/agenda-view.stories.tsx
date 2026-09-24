import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoEvents, demoPeriods, demoWeekendDays } from './_fixture';
import { AgendaView } from './agenda-view';

const meta = {
  title: 'Features/Calendar/AgendaView',
  component: AgendaView,
  args: {
    anchor: '2026-03-10',
    events: demoEvents,
    weekendDays: demoWeekendDays,
    periods: demoPeriods,
    canEdit: true,
    onSelectDate: () => {},
    onSelectEvent: () => {},
  },
} satisfies Meta<typeof AgendaView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const ReadOnly: Story = { args: { canEdit: false } };
export const NoEvents: Story = { args: { events: [] } };
