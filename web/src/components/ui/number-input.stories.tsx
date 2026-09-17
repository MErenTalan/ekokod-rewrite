import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';

import { NumberInput, type NumberInputProps } from './number-input';

function Stateful(props: Partial<NumberInputProps>) {
  const [value, setValue] = useState<string | null>(props.value ?? '1234.56');
  return <NumberInput label="Birim fiyat" unit="TRY" fractionDigits={2} {...props} value={value} onValueChange={setValue} />;
}

const meta = { title: 'UI/NumberInput', component: NumberInput, args: { label: '', value: null, onValueChange: () => {} } } satisfies Meta<typeof NumberInput>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const Energy: Story = { render: () => <Stateful label="Aylık tüketim" unit="kWh" fractionDigits={undefined} value="182345.125" /> };
export const Error: Story = { render: () => <Stateful error="Birim fiyat 0'dan büyük olmalı" value={null} /> };
export const Disabled: Story = { render: () => <Stateful disabled /> };
export const LongTurkishLabel: Story = { render: () => <Stateful label="Reaktif Endüktif Tüketim Oranı Eşik Değeri" unit="percent" value="20" /> };
