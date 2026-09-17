import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { BuildingComparison } from '@/lib/api/types';

import { SectorComparisonView } from './sector-comparison';

const metric = (value: string, average: string, rank: number, ranked: number) => ({ value, average, rank, ranked });
const comparison: BuildingComparison = {
  available: true,
  sector: 'Üretim',
  peers: 4,
  daily_consumption: metric('412.5', '380', 2, 4),
  monthly_consumption: metric('12375', '11400.25', 3, 4),
  co2_emission_kg: metric('5803.87', '5346.71', 3, 4),
  consumption_per_capita: metric('68.75', '61.5', 2, 4),
  consumption_per_area: metric('2.475', '2.28', 0, 0),
};

const meta = {
  title: 'Features/Dashboard/SectorComparison',
  component: SectorComparisonView,
  args: { buildingName: 'A1 Fabrika', comparison, canEditBuildings: true, onExport: () => {} },
} satisfies Meta<typeof SectorComparisonView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const TooFewPeers: Story = {
  args: { comparison: { ...comparison, available: false, reason: 'sector_too_small' } },
};
export const SectorMissing: Story = { args: { comparison: null, sectorMissing: true } };
export const NoBuilding: Story = { args: { buildingName: null, comparison: null } };
export const Loading: Story = { args: { loading: true } };
