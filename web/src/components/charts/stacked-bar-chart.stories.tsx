import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { empty } from './_story-data';
import { StackedBarChart } from './stacked-bar-chart';

const meta = {
  title: 'Charts/StackedBarChart',
  component: StackedBarChart,
  args: {
    title: 'Fatura bileşimi',
    description: 'Merkez Bina · son üç fatura',
    xLabel: 'Fatura dönemi',
    layout: 'horizontal',
    series: [
      { key: 'energy', label: 'Aktif enerji', kind: 'cost', unit: 'TRY' },
      { key: 'distribution', label: 'Dağıtım', kind: 'consumption', unit: 'TRY' },
      { key: 'reactive', label: 'Reaktif ceza', kind: 'reactiveInductive', unit: 'TRY' },
      { key: 'taxes', label: 'Vergi ve fonlar', kind: 'previous', unit: 'TRY' },
    ],
    data: [
      { x: 'Haziran 2026', energy: '382104.20', distribution: '71220.45', reactive: '0', taxes: '64812.10' },
      { x: 'Temmuz 2026', energy: '401882.75', distribution: '74108.30', reactive: '5515.10', taxes: '68990.40' },
      { x: 'Ağustos 2026', energy: '398241.74', distribution: '75126.19', reactive: '2210.00', taxes: '68103.62' },
    ],
    empty,
  },
} satisfies Meta<typeof StackedBarChart>;
export default meta;
type Story = StoryObj<typeof meta>;

export const InvoiceComposition: Story = {};
export const Scope123: Story = {
  args: {
    title: 'Kapsam 1, 2 ve 3 dağılımı',
    description: 'Şirket geneli · 2024–2026',
    xLabel: 'Yıl',
    layout: 'vertical',
    series: [
      { key: 's1', label: 'Kapsam 1', kind: 'cost', unit: 'tCO2e' },
      { key: 's2', label: 'Kapsam 2', kind: 'consumption', unit: 'tCO2e' },
      { key: 's3', label: 'Kapsam 3', kind: 'previous', unit: 'tCO2e' },
    ],
    data: [
      { x: '2024', s1: '214.5', s2: '598.2', s3: '1204' },
      { x: '2025', s1: '198.1', s2: '571.9', s3: '1150.5' },
      { x: '2026', s1: '176.4', s2: '512.3', s3: '1098.25' },
    ],
  },
};
