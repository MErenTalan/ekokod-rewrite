import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';

import { renderWithProviders } from '@/test/render';

import { demoAnalyzers, demoBill } from './_fixture';
import { BillHistoryView } from './bill-history';

const bills = [
  demoBill({ id: 'b-oct', scope: 'building', analyzer_id: undefined, period_key: '2026-10', total_cost: '655.01' }),
  demoBill({ id: 'b-aug' }),
];

describe('BillHistoryView', () => {
  it('lists every bill with its kind, building, meter and amount', () => {
    const r = renderWithProviders(
      <BillHistoryView bills={bills} buildingNames={{ 'b-1': 'Ev' }} analyzers={demoAnalyzers}
        onOpenMonth={vi.fn()} onDownloadPdf={vi.fn()} />,
    );
    const oct = r.getByRole('row', { name: /2026-10/ });
    expect(oct).toHaveTextContent('Bina faturası');
    expect(oct).toHaveTextContent('Ev');
    expect(oct).toHaveTextContent('655,01 TRY');
    expect(r.getByRole('row', { name: /2026-08/ })).toHaveTextContent('40001 · A1 Fabrika A.Ş.');
  });

  it('opens a month and downloads a bill', async () => {
    const user = userEvent.setup();
    const onOpenMonth = vi.fn();
    const onDownloadPdf = vi.fn();
    const r = renderWithProviders(
      <BillHistoryView bills={bills} buildingNames={{}} analyzers={[]} onOpenMonth={onOpenMonth} onDownloadPdf={onDownloadPdf} />,
    );
    await user.click(r.getByRole('button', { name: '2026-10 dönemini aç' }));
    expect(onOpenMonth).toHaveBeenCalledWith('2026-10');
    await user.click(r.getByRole('button', { name: '2026-08 faturasını indir' }));
    expect(onDownloadPdf).toHaveBeenCalledWith('b-aug');
  });

  it('says when there is no bill yet', () => {
    const r = renderWithProviders(
      <BillHistoryView bills={[]} buildingNames={{}} analyzers={[]} onOpenMonth={vi.fn()} onDownloadPdf={vi.fn()} />,
    );
    expect(r.getByText('Henüz fatura yok')).toBeVisible();
  });
});
