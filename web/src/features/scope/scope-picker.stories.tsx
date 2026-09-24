import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { ScopePickerView } from './scope-picker';

const buildings = [
  { id: 'b-1', name: 'A1 Fabrika', active: true },
  { id: 'b-2', name: 'A2 Depo', active: false },
];
const analyzers = [
  { id: 'a-1', buildingId: 'b-1', name: 'Merkez Ofis Analizör', active: true },
  { id: 'a-2', buildingId: 'b-1', name: '4001234567', active: true },
  { id: 'a-3', buildingId: 'b-2', name: '4009876543', active: false },
];

const meta = {
  title: 'Features/Scope/ScopePicker',
  component: ScopePickerView,
  args: {
    buildings,
    analyzers,
    value: { buildingId: 'b-1', analyzerId: 'a-1' },
    onValueChange: () => {},
    activeOnly: false,
    onActiveOnlyChange: () => {},
  },
} satisfies Meta<typeof ScopePickerView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const ActiveOnly: Story = { args: { activeOnly: true } };
export const Loading: Story = { args: { loading: true } };
