import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useTranslations } from 'next-intl';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { MultiSelect, type MultiSelectProps } from './multi-select';

const options = ['Merkez Bina', 'Soğuk Hava Deposu', 'İdari Ofis', 'Üretim Tesisi', 'GES Sahası'].map((label, i) => ({ value: `b${i}`, label }));

function Stateful(props: Partial<MultiSelectProps>) {
  const t = useTranslations('forms');
  const [value, setValue] = useState<string[]>(props.value ?? ['b0', 'b1', 'b3', 'b4']);
  return <MultiSelect label="Binalar" options={options} searchPlaceholder="Bina ara" emptyText={t('noResults')} maxChips={2} {...props} value={value} onValueChange={setValue} />;
}

const meta = {
  title: 'UI/MultiSelect',
  component: MultiSelect,
  args: { label: '', options, value: [], onValueChange: () => {}, searchPlaceholder: '', emptyText: '' },
} satisfies Meta<typeof MultiSelect>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const Error: Story = { render: () => <Stateful value={[]} error="En az bir bina seçin" /> };
export const Disabled: Story = { render: () => <Stateful disabled /> };
export const LongTurkishLabel: Story = { render: () => <Stateful label="Reaktif Endüktif Tüketim Oranı Eşik Değeri Binaları" /> };
export const Open: Story = {
  tags: ['open'],
  render: () => <Stateful />,
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('combobox'));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('listbox')).toBeInTheDocument();
  },
};
