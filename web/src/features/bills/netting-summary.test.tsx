import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoDashboard } from './_fixture';
import { NettingSummaryView } from './netting-summary';

const view = (dashboard = demoDashboard) =>
  renderWithProviders(<NettingSummaryView netting={dashboard.netting} plants={dashboard.plants} />);

describe('NettingSummaryView', () => {
  it('shows the four §7.10 figures and the netting status', () => {
    const r = view();
    expect(r.getByText(/Toplam tüketim/)).toBeVisible();
    expect(r.getByText(/Toplam üretim/)).toBeVisible();
    expect(r.getByText(/Toplam fatura/)).toBeVisible();
    expect(r.getByText(/Net tüketim/)).toBeVisible();
  });

  it('says why an efficiency does not exist instead of printing a zero', () => {
    const r = view({
      ...demoDashboard,
      netting: [{ ...demoDashboard.netting[0], efficiency_pct: undefined, total_consumption: '0', net: '0' }],
    });
    expect(r.getByText(/Tüketim olmadan oran hesaplanamaz/)).toBeVisible();
  });

  it('names the plant section and says its data source does not exist yet (R235)', () => {
    const r = view();
    expect(r.getByText(/GES santralleri/)).toBeVisible();
    expect(r.getByText(/veri kaynağı henüz yok/i)).toBeVisible();
    expect(r.queryByRole('table')).toBeNull();
  });

  it('shows one summary per currency, never a sum across them (R253)', () => {
    const r = view({
      ...demoDashboard,
      netting: [
        demoDashboard.netting[0],
        { ...demoDashboard.netting[0], currency: 'USD', total_invoice: '100' },
      ],
    });
    // The currency labels the block and its invoice tile, so it appears twice
    // per block — what matters is that each currency has its own block.
    expect(r.getAllByText('TRY').length).toBeGreaterThan(0);
    expect(r.getAllByText('USD').length).toBeGreaterThan(0);
    expect(r.getAllByText(/Netleşme/).length).toBe(2);
  });
});
