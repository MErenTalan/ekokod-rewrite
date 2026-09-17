import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';

import { Switch, type SwitchProps } from './switch';

function Stateful(props: Partial<SwitchProps>) {
  const [checked, setChecked] = useState(props.checked ?? true);
  return (
    <div className="max-w-sm">
      <Switch label="E-posta bildirimleri" {...props} checked={checked} onCheckedChange={setChecked} />
    </div>
  );
}

const meta = { title: 'UI/Switch', component: Switch, args: { label: '', checked: false, onCheckedChange: () => {} } } satisfies Meta<typeof Switch>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const Error: Story = { render: () => <Stateful checked={false} error="SMTP ayarları eksik" /> };
export const Disabled: Story = { render: () => <Stateful disabled description="Yöneticiniz kapattı" /> };
export const LongTurkishLabel: Story = { render: () => <Stateful label="Reaktif Endüktif Tüketim Oranı Eşik Değeri uyarıları" /> };
