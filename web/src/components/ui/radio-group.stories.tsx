import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';

import { RadioGroup, type RadioGroupProps } from './radio-group';

const options = [
  { value: 'hourly', label: 'Saatlik' },
  { value: 'daily', label: 'Günlük' },
  { value: 'monthly', label: 'Aylık' },
  { value: 'yearly', label: 'Yıllık', disabled: true },
];

function Stateful(props: Partial<RadioGroupProps>) {
  const [value, setValue] = useState(props.value ?? 'daily');
  return <RadioGroup label="Çözünürlük" options={options} {...props} value={value} onValueChange={setValue} />;
}

const meta = { title: 'UI/RadioGroup', component: RadioGroup, args: { label: '', options, value: '', onValueChange: () => {} } } satisfies Meta<typeof RadioGroup>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const Horizontal: Story = { render: () => <Stateful orientation="horizontal" /> };
export const Error: Story = { render: () => <Stateful error="Bir çözünürlük seçin" /> };
export const Disabled: Story = { render: () => <Stateful disabled /> };
export const LongTurkishLabel: Story = {
  render: () => <Stateful label="Reaktif Endüktif Tüketim Oranı Eşik Değeri" options={[{ value: 'a', label: 'Endüktif tüketimin aktif tüketime oranı' }, { value: 'b', label: 'Kapasitif tüketimin aktif tüketime oranı' }]} value="a" />,
};
