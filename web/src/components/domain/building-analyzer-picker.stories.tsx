import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';

import { BuildingAnalyzerPicker, type BuildingAnalyzerValue } from './building-analyzer-picker';

const buildings = [
  { id: 'a', name: 'İstanbul Merkez Bina', active: true },
  { id: 'b', name: 'Ankara Soğuk Hava Deposu', active: true },
  { id: 'c', name: 'İzmir Eski Depo', active: false },
];
const analyzers = [
  { id: 'a1', buildingId: 'a', name: 'Ana Dağıtım Panosu', active: true },
  { id: 'a2', buildingId: 'a', name: 'Kompresör Hattı', active: false },
  { id: 'b1', buildingId: 'b', name: 'Soğutma Grubu 1', active: true },
  { id: 'b2', buildingId: 'b', name: 'Soğutma Grubu 2', active: true },
];

function Stateful({ initial }: { initial: BuildingAnalyzerValue }) {
  const [value, setValue] = useState(initial);
  const [activeOnly, setActiveOnly] = useState(true);
  return <BuildingAnalyzerPicker buildings={buildings} analyzers={analyzers} value={value} onValueChange={setValue} activeOnly={activeOnly} onActiveOnlyChange={setActiveOnly} />;
}

const meta = {
  title: 'Domain/BuildingAnalyzerPicker',
  component: BuildingAnalyzerPicker,
  args: { buildings, analyzers, value: { buildingId: null, analyzerId: null }, onValueChange: () => {}, activeOnly: true, onActiveOnlyChange: () => {} },
} satisfies Meta<typeof BuildingAnalyzerPicker>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful initial={{ buildingId: 'b', analyzerId: 'b1' }} /> };
export const Empty: Story = { render: () => <Stateful initial={{ buildingId: null, analyzerId: null }} /> };
