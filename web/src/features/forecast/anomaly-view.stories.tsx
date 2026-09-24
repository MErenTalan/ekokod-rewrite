import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { AnomalyView } from './anomaly-view';

const verdict = {
  available: true, is_anomaly: true, score: 7.52, actual: '40', expected: '10', lower: '4.81', upper: '15.19',
  method: 'robust_zscore_same_hour_of_week', model_id: 'robust_zscore_same_hour_of_week', model_version: '1.0.0',
};

const meta = { title: 'Features/Forecast/AnomalyView', component: AnomalyView, args: { result: verdict } } satisfies Meta<typeof AnomalyView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Anomalous: Story = {};
export const Normal: Story = { args: { result: { ...verdict, is_anomaly: false, actual: '11', score: 0.67 } } };
export const InsufficientHistory: Story = { args: { result: { available: true, is_anomaly: false, actual: '40', method: 'insufficient_history' } } };
