import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { StatTile } from './stat-tile';

const meta = { title: 'UI/StatTile', component: StatTile, args: { label: 'Toplam tüketim', value: '182.345,1', unit: 'kWh' } } satisfies Meta<typeof StatTile>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { hint: 'Ağustos 2026' } };
export const Row: Story = {
  render: () => (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
      <StatTile label="Toplam tüketim" value="182.345,1" unit="kWh" />
      <StatTile label="Toplam maliyet" value="₺412.908,20" />
      <StatTile label="Karbon ayak izi" value="81,4" unit="tCO2e" />
    </div>
  ),
};
export const LongTurkishLabel: Story = { args: { label: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri', value: '18,2', unit: 'percent' } };
