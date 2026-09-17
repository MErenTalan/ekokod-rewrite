import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { Select, type SelectProps } from './select';

const options = [
  { value: 'hourly', label: 'Saatlik' },
  { value: 'daily', label: 'Günlük' },
  { value: 'monthly', label: 'Aylık' },
  { value: 'yearly', label: 'Yıllık', disabled: true },
];

function Stateful(props: Partial<SelectProps>) {
  const [value, setValue] = useState<string | null>(props.value ?? 'daily');
  return <Select label="Çözünürlük" options={options} placeholder="Seçin" {...props} value={value} onValueChange={setValue} />;
}

const meta = { title: 'UI/Select', component: Select, args: { label: '', options, value: null, onValueChange: () => {} } } satisfies Meta<typeof Select>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const Error: Story = { render: () => <Stateful value={null} error="Çözünürlük seçin" /> };
export const Disabled: Story = { render: () => <Stateful disabled /> };
export const LongTurkishLabel: Story = { render: () => <Stateful label="Reaktif Endüktif Tüketim Oranı Eşik Değeri" /> };
export const Open: Story = {
  tags: ['open', 'modal-listbox'],
  render: () => <Stateful />,
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('combobox'));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('listbox')).toBeInTheDocument();
  },
};
