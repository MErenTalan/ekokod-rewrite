import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { empty, MONTHS, monthly, wave } from './_story-data';
import type { ChartSeries, Datum } from './_theme';
import { LineChart } from './line-chart';

const yoy: ChartSeries[] = [
  { key: 'y2026', label: '2026', kind: 'current', unit: 'kWh' },
  { key: 'y2025', label: '2025', kind: 'previous', unit: 'kWh' },
];

const meta = {
  title: 'Charts/LineChart',
  component: LineChart,
  args: { title: 'Aylık elektrik tüketimi', description: 'Merkez Bina · 2025 ile karşılaştırma', xLabel: 'Ay', series: yoy, data: monthly({ y2026: [182000, 24000], y2025: [176000, 26000, 1] }), empty },
} satisfies Meta<typeof LineChart>;
export default meta;
type Story = StoryObj<typeof meta>;

export const ConsumptionVsLastYear: Story = {};

export const LoadProfile24h: Story = {
  args: {
    title: '24 saatlik yük profili',
    description: 'Hafta içi, cumartesi ve pazar ortalamaları',
    xLabel: 'Saat',
    formatX: (x) => `${String(x).padStart(2, '0')}:00`,
    series: [
      { key: 'weekday', label: 'Hafta içi', kind: 'consumption', unit: 'kW' },
      { key: 'saturday', label: 'Cumartesi', kind: 'consumption', unit: 'kW' },
      { key: 'sunday', label: 'Pazar', kind: 'consumption', unit: 'kW' },
    ],
    data: Array.from({ length: 24 }, (_, h) => ({ x: h, weekday: wave(h, 420, 180, 24, -6), saturday: wave(h, 300, 120, 24, -6), sunday: wave(h, 210, 60, 24, -6) })),
  },
};

export const ForecastWithBand: Story = {
  args: {
    title: 'Tüketim tahmini',
    description: 'Önümüzdeki 12 ay için medyan tahmin ve p10–p90 aralığı',
    series: [{ key: 'p50', label: 'Tahmin (medyan)', kind: 'forecast', unit: 'MWh' }],
    band: { lowerKey: 'p10', upperKey: 'p90', label: 'Tahmin aralığı (p10–p90)' },
    data: MONTHS.map((x, i) => ({ x, p50: wave(i, 180, 22), p10: wave(i, 160, 20), p90: wave(i, 204, 25) })),
  },
};

export const Loading: Story = { args: { loading: true } };
export const Empty: Story = { args: { data: [] } };
export const TooManySeries: Story = {
  args: { series: Array.from({ length: 7 }, (_, i): ChartSeries => ({ key: `b${i}`, label: `Bina ${i + 1}`, kind: 'consumption', unit: 'kWh' })) },
};
export const Downsampled: Story = {
  args: {
    title: '15 dakikalık tüketim',
    description: 'Ağustos 2026, 3.000 ölçüm',
    xLabel: 'Ölçüm',
    directLabels: false,
    series: [{ key: 'kw', label: 'Anlık güç', kind: 'consumption', unit: 'kW' }],
    data: Array.from({ length: 3000 }, (_, i): Datum => ({ x: i, kw: wave(i, 400, 150, 96) })),
  },
};
export const LongTurkishLabel: Story = {
  args: { title: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri ile aylık karşılaştırma', series: [{ ...yoy[0], label: 'Reaktif Endüktif Tüketim' }, yoy[1]] },
};
