import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';

import { Button } from '@/components/ui/button';

import { demoAnalyzers } from './_fixture';
import { AlarmDialogView } from './alarm-dialog';
import { emptyDraft, type AlarmDraft, type AlarmKind } from './alarm-draft';

/** The house pattern for a controlled dialog: the STORY owns the opener, so
 *  the dialog is reached the way a user reaches it (F6b's a11y finding 3). */
function Harness({ type }: { type: AlarmKind }) {
  const [draft, setDraft] = useState<AlarmDraft | null>(null);
  return (
    <>
      <Button onClick={() => setDraft({ ...emptyDraft(), type, name: 'Örnek kural', analyzerIds: ['a-1'] })}>
        Yeni Alarm Ekle
      </Button>
      {draft ? (
        <AlarmDialogView
          open
          draft={draft}
          analyzers={demoAnalyzers}
          onDraftChange={setDraft}
          onSubmit={() => setDraft(null)}
          onClose={() => setDraft(null)}
        />
      ) : null}
    </>
  );
}

const meta = {
  title: 'Features/Alarms/AlarmDialog',
  component: Harness,
  args: { type: 'reactive_limit' as AlarmKind },
} satisfies Meta<typeof Harness>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Reactive: Story = {};
export const DataCommunication: Story = { args: { type: 'data_communication' } };
/** R212: the power alarm has no voltage fields, because nothing reports voltage. */
export const Power: Story = { args: { type: 'current_voltage_power' } };
export const Invoice: Story = { args: { type: 'invoice_increase' } };
