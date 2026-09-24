import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';

import { Checkbox, type CheckboxProps } from './checkbox';

function Stateful(props: Partial<CheckboxProps>) {
  const [checked, setChecked] = useState<CheckboxProps['checked']>(props.checked ?? true);
  return <Checkbox label="Yalnızca aktif analizörler" {...props} checked={checked} onCheckedChange={setChecked} />;
}

const meta = { title: 'UI/Checkbox', component: Checkbox, args: { label: '', checked: false, onCheckedChange: () => {} } } satisfies Meta<typeof Checkbox>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const Indeterminate: Story = { render: () => <Stateful label="Tüm binalar" checked="indeterminate" /> };
export const Error: Story = { render: () => <Stateful checked={false} label="Kullanım koşullarını kabul ediyorum" error="Devam etmek için onaylayın" /> };
export const Disabled: Story = { render: () => <Stateful disabled /> };
export const LongTurkishLabel: Story = { render: () => <Stateful label="Reaktif Endüktif Tüketim Oranı Eşik Değeri aşıldığında e-posta gönder" /> };
