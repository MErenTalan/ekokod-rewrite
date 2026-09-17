import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { MapPanelView } from './map-panel';

const buildings = [
  { id: 'b-1', name: 'A1 Fabrika', lat: '39.925533', lng: '32.866287', status: 'active' as const },
  { id: 'b-2', name: 'A2 Depo', lat: '41.008238', lng: '28.978359', status: 'passive' as const },
  { id: 'b-3', name: 'Şantiye (konumsuz)', lat: null, lng: null, status: 'active' as const },
];
const analyzers = [
  { id: 'a-1', name: '4001234567', lat: '39.925533', lng: '32.866287', status: 'active' as const },
  { id: 'a-2', name: '4009876543', lat: null, lng: null, status: 'passive' as const },
];

const meta = {
  title: 'Features/Dashboard/MapPanel',
  component: MapPanelView,
  args: { buildings, analyzers, onSelectBuilding: () => {}, onSelectAnalyzer: () => {} },
} satisfies Meta<typeof MapPanelView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Focused: Story = { args: { focusId: 'b-2' } };
export const Loading: Story = { args: { loading: true } };
