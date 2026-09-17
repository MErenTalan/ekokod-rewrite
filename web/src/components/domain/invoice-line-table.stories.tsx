import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { InvoiceLineTable, type InvoiceLineView } from './invoice-line-table';

const lines: InvoiceLineView[] = [
  { id: '1', description: 'Aktif enerji bedeli', quantity: '182345.120', unit: 'kWh', unitPrice: '2.1840', amount: '398241.74', kind: 'energy' },
  { id: '2', description: 'Dağıtım bedeli', quantity: '182345.120', unit: 'kWh', unitPrice: '0.4120', amount: '75126.19', kind: 'distribution' },
  { id: '3', description: 'Reaktif enerji cezası', quantity: '4210.000', unit: 'kVArh', unitPrice: '1.3100', amount: '5515.10', kind: 'penalty' },
  { id: '4', description: 'Elektrik ve havagazı tüketim vergisi', quantity: null, unit: null, unitPrice: null, amount: '3982.42', kind: 'tax' },
  { id: '5', description: 'KDV (%20)', quantity: null, unit: null, unitPrice: null, amount: '96573.09', kind: 'tax' },
];

const meta = {
  title: 'Domain/InvoiceLineTable',
  component: InvoiceLineTable,
  args: { caption: 'Merkez Bina · Ağustos 2026 fatura kalemleri', currency: 'TRY', lines, totals: [{ label: 'KDV hariç toplam', amount: '482865.45' }, { label: 'Ödenecek tutar', amount: '579438.54' }] },
} satisfies Meta<typeof InvoiceLineTable>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { lines: [], totals: [] } };
