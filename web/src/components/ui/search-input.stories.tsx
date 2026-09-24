import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { SearchInput, type SearchInputProps } from './search-input';

function Stateful(props: Partial<SearchInputProps>) {
  const t = useTranslations('common');
  const [value, setValue] = useState(props.value ?? 'Merkez');
  return <SearchInput label={t('search')} placeholder="Bina veya analizör ara" {...props} value={value} onValueChange={setValue} />;
}

const meta = { title: 'UI/SearchInput', component: SearchInput, args: { label: '', value: '', onValueChange: () => {} } } satisfies Meta<typeof SearchInput>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const HiddenLabel: Story = { render: () => <Stateful labelVisibility="hidden" value="" /> };
export const Disabled: Story = { render: () => <Stateful disabled /> };
export const LongTurkishLabel: Story = { render: () => <Stateful label="Reaktif Endüktif Tüketim Oranı Eşik Değeri ara" /> };
