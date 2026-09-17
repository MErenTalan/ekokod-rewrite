import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Sparkline } from '../charts/sparkline';
import { MetricCard } from './metric-card';

const trend = Array.from({ length: 30 }, (_, i) => (6000 + 1400 * Math.sin(i / 1.1)).toFixed(1));

const meta = {
  title: 'Domain/MetricCard',
  component: MetricCard,
  args: {
    label: 'Toplam tüketim',
    value: '182345.12',
    unit: 'kWh',
    delta: { value: '12.5', direction: 'up', sentiment: 'bad', comparisonLabel: 'geçen aya göre' },
    sparkline: <Sparkline label="Son 30 gün" kind="consumption" data={trend} />,
  },
} satisfies Meta<typeof MetricCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const CostDown: Story = { args: { label: 'Toplam maliyet', value: '412908.2', unit: 'TRY', sparkline: undefined, delta: { value: '-3.2', direction: 'down', sentiment: 'good', comparisonLabel: 'geçen yıla göre' } } };
export const Estimated: Story = { args: { quality: { state: 'estimated', reason: '6 saatlik ölçüm tahminle dolduruldu', coverage: '99.2' } } };
export const NoValue: Story = { args: { value: null, delta: undefined, sparkline: undefined, quality: { state: 'incomplete', reason: 'Analizör veri göndermiyor' } } };
export const Loading: Story = { args: { loading: true } };
export const LongTurkishLabel: Story = { args: { label: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri', value: '18.2', unit: 'percent', sparkline: undefined, delta: { value: '0', direction: 'flat', sentiment: 'neutral', comparisonLabel: 'geçen dönemle aynı' } } };
