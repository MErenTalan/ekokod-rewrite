import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { Button } from '@/components/ui/button';

import { activity, factor } from './_fixture';
import { ActivityDialog, type ActivityDialogProps } from './activity-dialog';

const sub = { key: 'sub_space_heating', scope: 'scope_1' as const, iso_category: 'category_1' };
const factors = [factor(), factor({ id: 'f-lpg', key: 'lpg', label: 'Ortam Isıtması > LPG', category_path: ['lpg'], base_unit: 'litre', conversions: [] })];

/** The dialog behind its trigger, as on the screen: the canvas is never empty. */
function Example(props: Partial<ActivityDialogProps>) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Veri ekle</Button>
      <ActivityDialog sub={sub} factors={factors} today="2026-09-10" errors={{}} saving={false} onSubmit={() => setOpen(false)}
        {...props} open={open} onClose={() => setOpen(false)} />
    </>
  );
}

const meta = {
  title: 'Features/Carbon/ActivityDialog',
  component: ActivityDialog,
  args: { open: false, sub, factors, today: '2026-09-10', errors: {}, saving: false, onSubmit: () => {}, onClose: () => {} },
} satisfies Meta<typeof ActivityDialog>;
export default meta;
type Story = StoryObj<typeof meta>;

const openIt: Story['play'] = async ({ canvasElement }) => {
  await userEvent.click(within(canvasElement).getByRole('button'));
  await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
};

export const Add: Story = { tags: ['open'], render: () => <Example />, play: openIt };
export const Edit: Story = { tags: ['open'], render: () => <Example initial={activity()} />, play: openIt };
export const ServerRefusal: Story = {
  tags: ['open'],
  render: () => <Example initial={activity()} errors={{ quantity: 'Miktar 0’dan büyük olmalı.' }} />,
  play: openIt,
};
