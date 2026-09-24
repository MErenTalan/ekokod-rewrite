import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Textarea } from './textarea';

const meta = { title: 'UI/Textarea', component: Textarea, args: { label: 'Fatura notu' } } satisfies Meta<typeof Textarea>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { defaultValue: 'Ağustos faturasında reaktif ceza itiraz edildi.' } };
export const Error: Story = { args: { error: 'Not en fazla 500 karakter olabilir' } };
export const Disabled: Story = { args: { disabled: true, defaultValue: 'Kilitli dönem' } };
export const LongTurkishLabel: Story = { args: { label: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri Açıklaması' } };
