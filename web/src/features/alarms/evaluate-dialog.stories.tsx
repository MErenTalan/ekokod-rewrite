import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import type { AlarmEvaluation } from '@/lib/api/types';

import { demoEvaluation } from './_fixture';
import { EvaluateDialogView } from './evaluate-dialog';

function Harness({ evaluation }: { evaluation: AlarmEvaluation | null }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Şimdi değerlendir</Button>
      <EvaluateDialogView
        open={open}
        alarmName="Endüktif izleme"
        evaluation={evaluation}
        loading={evaluation === null}
        onClose={() => setOpen(false)}
      />
    </>
  );
}

const meta = {
  title: 'Features/Alarms/EvaluateDialog',
  component: Harness,
  args: { evaluation: demoEvaluation },
} satisfies Meta<typeof Harness>;
export default meta;
type Story = StoryObj<typeof meta>;

/** One analyzer fired, one could not be decided — the two are rendered
 *  differently on purpose (R216, R219). */
export const Default: Story = {};
export const Loading: Story = { args: { evaluation: null } };
