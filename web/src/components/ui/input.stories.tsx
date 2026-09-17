import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Input } from './input';

const meta = { title: 'UI/Input', component: Input, args: { label: 'Bina adı' } } satisfies Meta<typeof Input>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { defaultValue: 'Merkez Bina', description: 'Raporlarda görünen ad' } };
export const Error: Story = { args: { required: true, error: 'Bina adı zorunludur' } };
export const Disabled: Story = { args: { disabled: true, defaultValue: 'Merkez Bina' } };
export const LongTurkishLabel: Story = { args: { label: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri', defaultValue: '%20' } };
