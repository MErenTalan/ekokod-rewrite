import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { forecast } from './_fixture';
import { PredictionResult } from './prediction-result';

const rows = forecast.points.slice(0, 6).map((p, i) => ({ label: `${String(i).padStart(2, '0')}:00`, median: p.median, p10: p.p10, p90: p.p90 }));

const meta = {
  title: 'Features/Forecast/PredictionResult',
  component: PredictionResult,
  args: { title: 'Günlük tahmin', points: forecast.points, rows, firstColumn: 'Saat', summary: 'Perşembe için toplam tahmin: 118 kWh · Yaklaşık aralık (saatlik bantların toplamı): 70 – 166' },
} satisfies Meta<typeof PredictionResult>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Daily: Story = {};
export const WeeklyNotStored: Story = { args: { title: 'Haftalık tahmin', firstColumn: 'Gün', summary: undefined, notStored: true } };
