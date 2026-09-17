import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useTranslations } from 'next-intl';
import { useState } from 'react';
import { expect, userEvent, within } from 'storybook/test';

import { type DateRange, DateRangePicker, type DateRangePickerProps } from './date-range-picker';

function Stateful(props: Partial<DateRangePickerProps>) {
  const t = useTranslations('forms');
  const [value, setValue] = useState<DateRange | null>(props.value === undefined ? { from: '2026-09-01', to: '2026-09-17' } : props.value);
  const presets = [
    { id: 'last7', label: t('last7Days'), range: { from: '2026-09-11', to: '2026-09-17' } },
    { id: 'lastMonth', label: t('lastMonth'), range: { from: '2026-08-01', to: '2026-08-31' } },
    { id: 'thisYear', label: t('thisYear'), range: { from: '2026-01-01', to: '2026-09-17' } },
  ];
  return <DateRangePicker label="Dönem" presets={presets} {...props} value={value} onValueChange={setValue} />;
}

const meta = { title: 'UI/DateRangePicker', component: DateRangePicker, args: { label: '', value: null, onValueChange: () => {} } } satisfies Meta<typeof DateRangePicker>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const Empty: Story = { render: () => <Stateful value={null} /> };
export const Error: Story = { render: () => <Stateful value={null} error="Dönem seçin" /> };
export const Disabled: Story = { render: () => <Stateful disabled /> };
export const LongTurkishLabel: Story = { render: () => <Stateful label="Reaktif Endüktif Tüketim Oranı Eşik Değeri karşılaştırma dönemi" /> };
export const Open: Story = {
  tags: ['open'],
  render: () => <Stateful />,
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button'));
    await expect((await within(canvasElement.ownerDocument.body).findAllByRole('grid')).length).toBe(2);
  },
};
