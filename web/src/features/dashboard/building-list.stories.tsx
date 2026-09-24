import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { BuildingListView } from './building-list';

const buildings = [
  { id: 'b-1', name: 'A1 Fabrika', address: 'Ostim OSB, Ankara', analyzerCount: 2, status: 'active' as const },
  { id: 'b-2', name: 'A2 Depo', address: 'Tuzla, İstanbul', analyzerCount: 1, status: 'passive' as const },
];
const analyzers = [
  { id: 'a-1', buildingId: 'b-1', name: 'Merkez Ofis Analizör' },
  { id: 'a-2', buildingId: 'b-1', name: '4001234567' },
  { id: 'a-3', buildingId: 'b-2', name: '4009876543' },
];

const meta = {
  title: 'Features/Dashboard/BuildingList',
  component: BuildingListView,
  args: {
    buildings,
    analyzers,
    selectedBuildingId: 'b-1',
    selectedAnalyzerId: 'a-1',
    visibleIds: ['b-1', 'b-2'],
    onVisibleChange: () => {},
    onSelectBuilding: () => {},
    onSelectAnalyzer: () => {},
    onFocus: () => {},
  },
} satisfies Meta<typeof BuildingListView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { buildings: [], analyzers: [] } };
export const Loading: Story = { args: { loading: true } };
