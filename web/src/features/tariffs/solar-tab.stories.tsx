import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoSolarTariffs } from './_fixture';
import { SolarTabView } from './solar-tab';

const meta = {
  title: 'Features/Tariffs/SolarTab',
  component: SolarTabView,
  args: {
    plants: [{ id: 'p-1', name: 'Çatı GES' }],
    plantID: 'p-1',
    onPlantChange: () => {},
    tariffs: demoSolarTariffs,
    canEdit: true,
    onCreate: () => {},
    onDelete: () => {},
  },
} satisfies Meta<typeof SolarTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const NoPlantChosen: Story = { args: { plantID: null } };
export const Empty: Story = { args: { tariffs: [] } };
export const ReadOnly: Story = { args: { canEdit: false } };
