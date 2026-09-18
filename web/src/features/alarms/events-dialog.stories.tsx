import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import type { Alarm } from '@/lib/api/types';

import { demoAlarms, demoEvents } from './_fixture';
import { EventsDialogView } from './events-dialog';

function Harness({ alarm, events }: { alarm: Alarm; events: typeof demoEvents }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Detaylar</Button>
      <EventsDialogView open={open} alarm={alarm} events={events} onClose={() => setOpen(false)} />
    </>
  );
}

const meta = {
  title: 'Features/Alarms/EventsDialog',
  component: Harness,
  args: { alarm: demoAlarms[0], events: demoEvents },
} satisfies Meta<typeof Harness>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { events: [] } };
