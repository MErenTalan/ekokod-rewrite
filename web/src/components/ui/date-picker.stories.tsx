import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { DatePicker, type DatePickerProps } from './date-picker';

function Stateful(props: Partial<DatePickerProps>) {
  const [value, setValue] = useState<string | null>(props.value === undefined ? '2026-09-17' : props.value);
  return <DatePicker label="Okuma tarihi" {...props} value={value} onValueChange={setValue} />;
}

const meta = { title: 'UI/DatePicker', component: DatePicker, args: { label: '', value: null, onValueChange: () => {} } } satisfies Meta<typeof DatePicker>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const Empty: Story = { render: () => <Stateful value={null} /> };
export const Error: Story = { render: () => <Stateful value={null} error="Tarih seçin" /> };
export const Disabled: Story = { render: () => <Stateful disabled /> };
export const LongTurkishLabel: Story = { render: () => <Stateful label="Reaktif Endüktif Tüketim Oranı Eşik Değeri geçerlilik tarihi" /> };
export const Open: Story = {
  tags: ['open'],
  render: () => <Stateful min="2026-09-05" max="2026-09-25" />,
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button'));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('grid')).toBeInTheDocument();
  },
};
