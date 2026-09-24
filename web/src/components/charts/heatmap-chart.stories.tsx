import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { empty, WEEKDAYS, wave } from './_story-data';
import { HeatmapChart } from './heatmap-chart';

const hours = Array.from({ length: 24 }, (_, h) => String(h).padStart(2, '0'));
const values = WEEKDAYS.map((_, d) => hours.map((_, h) => (d === 2 && h > 9 && h < 13 ? null : wave(h, d > 4 ? 180 : 360, d > 4 ? 60 : 170, 24, -6))));

const meta = {
  title: 'Charts/HeatmapChart',
  component: HeatmapChart,
  args: { title: 'Saat ve güne göre tüketim', description: 'Merkez Bina · Ağustos 2026 ortalaması', xLabel: 'Gün / saat', rows: WEEKDAYS, columns: hours, values, unit: 'kWh', kind: 'consumption', empty },
} satisfies Meta<typeof HeatmapChart>;
export default meta;
type Story = StoryObj<typeof meta>;

export const HourByWeekday: Story = {};
export const Empty: Story = { args: { values: WEEKDAYS.map(() => hours.map(() => null)) } };
