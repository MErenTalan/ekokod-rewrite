import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';

import { renderWithProviders } from '@/test/render';

import { demoDashboard, demoDivergentDashboard } from './_fixture';
import { DashboardTableView } from './dashboard-table';

const view = (dashboard = demoDashboard, over: Partial<Parameters<typeof DashboardTableView>[0]> = {}) =>
  renderWithProviders(<DashboardTableView buildings={dashboard.buildings} {...over} />);

describe('DashboardTableView', () => {
  it('renders every §7.10 column for a row', () => {
    const r = view();
    // The building name repeats on every row of its section plus its subtotal.
    expect(r.getAllByRole('cell', { name: 'A1 Fabrika' }).length).toBeGreaterThan(1);
    expect(r.getByRole('cell', { name: 'Sayaç 1' })).toBeVisible();
    expect(r.getByRole('cell', { name: '40001' })).toBeVisible();
    expect(r.getByRole('cell', { name: '40Z0000000001A' })).toBeVisible();
    expect(r.getAllByRole('cell', { name: '2026-08' }).length).toBe(2);
  });

  it('shows a building total that is the sum of the rows above it', () => {
    const r = view();
    const total = r.getByRole('row', { name: /Ara toplam/ });
    expect(total).toHaveTextContent('510');
    expect(total).toHaveTextContent('201');
  });

  it('shows the building invoice and says when it differs from the rows', () => {
    const r = view(demoDivergentDashboard);
    expect(r.getByRole('row', { name: /Bina faturası/ })).toHaveTextContent('495');
    expect(r.getByRole('note')).toHaveTextContent(/bina tarifesi toplam tüketime bir kez uygulanır/i);
  });

  it('does not cry divergence when the invoice agrees with its rows', () => {
    expect(view().queryByRole('note')).toBeNull();
  });

  it('downloads a row invoice and its hourly detail', async () => {
    const user = userEvent.setup();
    const onDownloadPdf = vi.fn();
    const onDownloadHourly = vi.fn();
    const r = view(demoDashboard, { onDownloadPdf, onDownloadHourly });
    await user.click(r.getAllByRole('button', { name: /Fatura PDF/ })[0]);
    await user.click(r.getAllByRole('button', { name: /Saatlik PTF/ })[0]);
    expect(onDownloadPdf).toHaveBeenCalledWith('bill-1');
    expect(onDownloadHourly).toHaveBeenCalledWith('bill-1');
  });

  it('shows the empty state when the month has no invoice', () => {
    const r = view({ ...demoDashboard, buildings: [] });
    expect(r.getByText(/Seçilen ayda fatura yok/)).toBeVisible();
  });
});
