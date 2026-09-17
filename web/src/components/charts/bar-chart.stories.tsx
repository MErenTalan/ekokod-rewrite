import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { empty, MONTHS, monthly, wave } from './_story-data';
import { BarChart } from './bar-chart';

const meta = {
  title: 'Charts/BarChart',
  component: BarChart,
  args: {
    title: 'Yıllık karşılaştırma',
    description: 'Merkez Bina · 2026 ve 2025',
    xLabel: 'Ay',
    series: [
      { key: 'y2026', label: '2026', kind: 'current', unit: 'kWh' },
      { key: 'y2025', label: '2025', kind: 'previous', unit: 'kWh' },
    ],
    data: monthly({ y2026: [182000, 24000], y2025: [176000, 26000, 1] }),
    empty,
  },
} satisfies Meta<typeof BarChart>;
export default meta;
type Story = StoryObj<typeof meta>;

export const YearOverYear: Story = {};

export const EmissionsByCategory: Story = {
  args: {
    title: 'Kategoriye göre emisyon',
    description: '2026 · kapsam 1 ve 2',
    xLabel: 'Kategori',
    layout: 'horizontal',
    sort: 'desc',
    series: [{ key: 't', label: 'Emisyon', kind: 'cost', unit: 'tCO2e' }],
    data: [
      { x: 'Şebeke elektriği', t: '412.8' },
      { x: 'Doğal gaz', t: '138.25' },
      { x: 'Şirket araçları', t: '64.1' },
      { x: 'Jeneratör', t: '12.4' },
      { x: 'Soğutucu gaz kaçağı', t: '9.75' },
    ],
  },
};

export const ImportExportBalance: Story = {
  args: {
    title: 'Şebeke çekiş / veriş dengesi',
    description: 'Pozitif değer şebekeden çekiş, negatif değer şebekeye veriş',
    diverging: true,
    series: [
      { key: 'net', label: 'Şebekeden çekiş', kind: 'consumption', unit: 'kWh' },
      { key: 'net', label: 'Şebekeye veriş', kind: 'generation', unit: 'kWh' },
    ],
    data: MONTHS.map((x, i) => ({ x, net: wave(i, 2000, 9000, 12, 3) })),
  },
};

export const TargetVsActual: Story = {
  args: {
    title: 'Hedef ve gerçekleşen üretim',
    description: 'GES Sahası · 2026',
    series: [
      { key: 'actual', label: 'Gerçekleşen', kind: 'generation', unit: 'MWh' },
      { key: 'target', label: 'Hedef', kind: 'target', unit: 'MWh' },
    ],
    data: monthly({ actual: [410, 160, -3], target: [430, 150, -3] }),
    referenceLine: { value: 450, label: 'Aylık sözleşme hedefi 450 MWh' },
  },
};

export const Loading: Story = { args: { loading: true } };
