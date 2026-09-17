import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useTranslations } from 'next-intl';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { Combobox, type ComboboxProps } from './combobox';

const options = [
  { value: 'ist', label: 'İstanbul Ofis' },
  { value: 'ank', label: 'Ankara Fabrika' },
  { value: 'izm', label: 'İzmir Soğuk Hava Deposu' },
  { value: 'bur', label: 'Bursa GES Sahası', disabled: true },
];

function Stateful(props: Partial<ComboboxProps>) {
  const t = useTranslations('forms');
  const [value, setValue] = useState<string | null>(props.value === undefined ? 'ank' : props.value);
  return <Combobox label="Bina" options={options} searchPlaceholder="Bina ara" emptyText={t('noResults')} {...props} value={value} onValueChange={setValue} />;
}

const meta = {
  title: 'UI/Combobox',
  component: Combobox,
  args: { label: '', options, value: null, onValueChange: () => {}, searchPlaceholder: '', emptyText: '' },
} satisfies Meta<typeof Combobox>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const Empty: Story = { render: () => <Stateful value={null} /> };
export const Error: Story = { render: () => <Stateful value={null} error="Bina seçin" /> };
export const Disabled: Story = { render: () => <Stateful disabled /> };
export const LongTurkishLabel: Story = { render: () => <Stateful label="Reaktif Endüktif Tüketim Oranı Eşik Değeri Binası" value="izm" /> };
export const Open: Story = {
  tags: ['open'],
  render: () => <Stateful />,
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('combobox'));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('listbox')).toBeInTheDocument();
  },
};
