import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { ProgressBar } from './progress-bar';

const meta = { title: 'UI/ProgressBar', component: ProgressBar, args: { label: 'Endüktif oran', value: 14, valueText: '%14' } } satisfies Meta<typeof ProgressBar>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { max: 30, thresholds: [{ value: 20, label: 'Sınır %20' }] } };
export const OverThreshold: Story = { args: { value: 25, valueText: '%25', max: 30, tone: 'danger', thresholds: [{ value: 20, label: 'Sınır %20' }] } };
export const Indeterminate: Story = { args: { label: 'Rapor hazırlanıyor', value: null, valueText: undefined } };
export const LongTurkishLabel: Story = { args: { label: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri', max: 30, thresholds: [{ value: 20, label: 'Sınır %20' }] } };
