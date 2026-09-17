import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { InvoiceLineTable, type InvoiceLineView } from './invoice-line-table';

const lines: InvoiceLineView[] = [
  { id: '1', description: 'Aktif enerji', quantity: '182345.12', unit: 'kWh', unitPrice: '2.1840', amount: '398241.74', kind: 'energy' },
  { id: '2', description: 'Reaktif ceza', quantity: null, unit: null, unitPrice: null, amount: '1234.56', kind: 'penalty' },
];

describe('InvoiceLineTable', () => {
  it('amounts are Turkish-formatted and totals in tfoot', () => {
    const { getByText, getByRole, container } = renderWithProviders(
      <InvoiceLineTable caption="Fatura kalemleri" currency="TRY" lines={lines} totals={[{ label: 'Genel toplam', amount: '478883.03' }]} />,
    );
    expect(getByRole('table', { name: 'Fatura kalemleri' })).toBeInTheDocument();
    expect(getByText('₺1.234,56')).toBeInTheDocument();
    expect(getByText('₺2,1840')).toBeInTheDocument();
    expect(container.querySelector('tfoot')).toHaveTextContent('Genel toplam');
    expect(container.querySelector('tfoot')).toHaveTextContent('₺478.883,03');
    expect(getByText('Ceza')).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<InvoiceLineTable caption="Kalemler" currency="TRY" lines={lines} totals={[]} />);
    await expectNoAxeViolations(container);
  });
});
