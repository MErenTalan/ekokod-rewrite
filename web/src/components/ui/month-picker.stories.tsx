import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { MonthPicker, type MonthPickerProps } from './month-picker';

function Stateful(props: Partial<MonthPickerProps>) {
  const [value, setValue] = useState<string | null>(props.value === undefined ? '2026-08' : props.value);
  return <MonthPicker label="Fatura dönemi" {...props} value={value} onValueChange={setValue} />;
}

const meta = { title: 'UI/MonthPicker', component: MonthPicker, args: { label: '', value: null, onValueChange: () => {} } } satisfies Meta<typeof MonthPicker>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const Error: Story = { render: () => <Stateful value={null} error="Dönem seçin" /> };
export const Disabled: Story = { render: () => <Stateful disabled /> };
export const LongTurkishLabel: Story = { render: () => <Stateful label="Reaktif Endüktif Tüketim Oranı Eşik Değeri fatura dönemi" /> };
export const Open: Story = {
  tags: ['open'],
  render: () => <Stateful min="2026-03" max="2026-09" />,
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button'));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('dialog')).toBeInTheDocument();
  },
};
