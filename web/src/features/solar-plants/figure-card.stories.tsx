import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { FigureCard } from './figure-card';

const meta = {
  title: 'Features/SolarPlants/FigureCard',
  component: FigureCard,
  args: { label: 'Anlık güç', value: '125.4', unit: 'kW' },
} satisfies Meta<typeof FigureCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Value: Story = {};
export const Missing: Story = { args: { value: undefined } };
