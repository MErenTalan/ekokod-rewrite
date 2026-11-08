import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoAnalyzers, demoPreview, demoSavedPeriod } from './_fixture';
import { CustomPeriodCardView } from './custom-period-card';

const meta = {
  title: 'Features/Bills/CustomPeriodCard',
  component: CustomPeriodCardView,
  args: { analyzers: demoAnalyzers, buildingId: 'b-1', onPreview: () => {}, onSave: () => {}, onDelete: () => {} },
} satisfies Meta<typeof CustomPeriodCardView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
/** A preview over the utility's 16:00 reading window, with days the meter did not report. */
export const WithPreview: Story = { args: { preview: demoPreview, savedPeriods: [demoSavedPeriod] } };
export const NoBuilding: Story = { args: { buildingId: null } };
export const FieldError: Story = { args: { errors: { to: 'Geçersiz değer' } } };
