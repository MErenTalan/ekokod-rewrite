import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoTariffSummaries } from './_fixture';
import { TariffHistoryView } from './tariff-history';

const meta = {
  title: 'Features/Tariffs/TariffHistory',
  component: TariffHistoryView,
  args: {
    tariffs: demoTariffSummaries,
    canEdit: true,
    onCreate: () => {},
    onEdit: () => {},
    onDelete: () => {},
  },
} satisfies Meta<typeof TariffHistoryView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
/** A company-readonly admin holds tariffs.read and not tariffs.edit (R246). */
export const ReadOnly: Story = { args: { canEdit: false } };
export const Empty: Story = { args: { tariffs: [] } };
export const Loading: Story = { args: { loading: true } };
