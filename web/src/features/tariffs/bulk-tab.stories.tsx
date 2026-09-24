import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoAssignments, demoBuildingStates } from './_fixture';
import { BulkTabView } from './bulk-tab';

const meta = {
  title: 'Features/Tariffs/BulkTab',
  component: BulkTabView,
  args: { buildings: demoBuildingStates, assignments: demoAssignments, canEdit: true, onAssign: () => {} },
} satisfies Meta<typeof BulkTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
/** A building with no tariff is a row, not an omission. */
export const WithoutTariffs: Story = {
  args: { buildings: demoBuildingStates.map((b) => ({ ...b, tariff_id: undefined, tariff_name: undefined, effective_from: undefined })) },
};
export const ReadOnly: Story = { args: { canEdit: false } };
export const NoHistory: Story = { args: { assignments: [] } };
