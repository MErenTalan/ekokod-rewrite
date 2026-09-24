import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoDashboard } from './_fixture';
import { PlantsSectionView } from './plants-section';

const meta = {
  title: 'Features/Bills/PlantsSection',
  component: PlantsSectionView,
  args: { plants: demoDashboard.plants },
} satisfies Meta<typeof PlantsSectionView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const NoPlants: Story = { args: { plants: { available: true, rows: [], total_invoice: [] } } };
export const TwoCurrencies: Story = {
  args: { plants: { ...demoDashboard.plants, total_invoice: [{ currency: 'TRY', amount: '2160' }, { currency: 'USD', amount: '10' }] } },
};
