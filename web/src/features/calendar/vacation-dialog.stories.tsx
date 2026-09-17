import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoPeriods, demoWeekendDays } from './_fixture';
import { VacationDialogView } from './vacation-dialog';

const meta = {
  title: 'Features/Calendar/VacationDialog',
  component: VacationDialogView,
  tags: ['open'],
  args: { open: true, onOpenChange: () => {}, weekendDays: demoWeekendDays, periods: demoPeriods, onSave: () => {} },
} satisfies Meta<typeof VacationDialogView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const EveryDayNonWorking: Story = { args: { weekendDays: [0, 1, 2, 3, 4, 5, 6] } };
export const ReadOnly: Story = { args: { readOnly: true } };
