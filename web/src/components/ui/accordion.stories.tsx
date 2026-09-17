import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Accordion } from './accordion';

const items = [
  { value: 'reactive', title: 'Reaktif ceza nasıl hesaplanır?', content: 'Endüktif tüketimin aktif tüketime oranı %20, kapasitif oran %15 sınırını aşarsa ceza uygulanır.' },
  { value: 'period', title: 'Fatura dönemi ne zaman kapanır?', content: 'Dönem, sayaç okuma tarihinde İstanbul saatiyle gece yarısı kapanır.' },
];

const meta = { title: 'UI/Accordion', component: Accordion, args: { items, type: 'single' } } satisfies Meta<typeof Accordion>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Multiple: Story = { args: { type: 'multiple' } };
export const LongTurkishLabel: Story = { args: { items: [{ value: 'a', title: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri nasıl belirlenir?', content: 'EPDK mevzuatı.' }] } };
