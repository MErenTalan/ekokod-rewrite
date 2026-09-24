import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';

import { formatNumber } from '@/lib/format';

import { Slider, type SliderProps } from './slider';

function Stateful(props: Partial<SliderProps>) {
  const [value, setValue] = useState(props.value ?? 1);
  return (
    <div className="max-w-sm">
      <Slider label="Köşe yuvarlaklığı" min={0.5} max={1.5} step={0.25} formatValue={(v) => `${formatNumber(v)}×`} {...props} value={value} onValueChange={setValue} />
    </div>
  );
}

const meta = { title: 'UI/Slider', component: Slider, args: { label: '', value: 0, onValueChange: () => {}, min: 0, max: 1, step: 1 } } satisfies Meta<typeof Slider>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const Error: Story = { render: () => <Stateful error="Değer aralık dışında" /> };
export const Disabled: Story = { render: () => <Stateful disabled /> };
export const LongTurkishLabel: Story = {
  render: () => <Stateful label="Reaktif Endüktif Tüketim Oranı Eşik Değeri" min={0} max={50} step={1} value={20} formatValue={(v) => `%${v}`} />,
};
