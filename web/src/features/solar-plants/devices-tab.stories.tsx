import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoDevices } from './_fixture';
import { DevicesTabView } from './devices-tab';

const meta = {
  title: 'Features/SolarPlants/Devices',
  component: DevicesTabView,
  args: { devices: demoDevices, query: '', onQueryChange: () => {} },
} satisfies Meta<typeof DevicesTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { devices: [] } };
