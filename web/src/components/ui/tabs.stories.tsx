import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Tabs } from './tabs';

const items = [
  { value: 'summary', label: 'Özet', content: <p className="text-foreground">Ağustos 2026 özet tablosu</p> },
  { value: 'hourly', label: 'Saatlik', content: <p className="text-foreground">Saatlik tüketim tablosu</p> },
  { value: 'stats', label: 'İstatistikler', content: <p className="text-foreground">İstatistikler</p> },
  { value: 'ai', label: 'Yapay Zeka', content: null, disabled: true },
];

const meta = { title: 'UI/Tabs', component: Tabs, args: { items } } satisfies Meta<typeof Tabs>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const LongTurkishLabel: Story = {
  args: { items: [{ value: 'a', label: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri', content: <p className="text-foreground">İçerik</p> }, ...items.slice(0, 2)] },
};
