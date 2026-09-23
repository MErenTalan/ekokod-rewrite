import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { within } from '@testing-library/react';

import { renderWithProviders } from '@/test/render';

import { demoDashboard } from './_fixture';
import { PlantsSectionView } from './plants-section';

describe('PlantsSectionView', () => {
  it('lists one row per plant with production, prices and sale value (R290)', () => {
    const r = renderWithProviders(<PlantsSectionView plants={demoDashboard.plants} />);
    const table = within(r.getByRole('region', { name: 'GES santralleri' })).getByRole('table');
    const rows = within(table).getAllByRole('row');
    expect(within(rows[1]).getByText('Konya GES')).toBeVisible();
    expect(within(rows[1]).getByText('Mahsup Sayacı')).toBeVisible();
    expect(within(rows[1]).getByText('1.200')).toBeVisible();
    expect(within(rows[1]).getByText('2.160,00')).toBeVisible();
    expect(within(rows[1]).getByText('TRY')).toBeVisible();
  });

  it('shows missing figures as "veri yok" and a plant without a netting analyzer as a dash', () => {
    const r = renderWithProviders(<PlantsSectionView plants={demoDashboard.plants} />);
    const row = r.getByText('Arazi GES').closest('tr') as HTMLElement;
    expect(within(row).getAllByText('—').length).toBe(3); // analyzer, installation no, currency
    expect(within(row).getAllByText('veri yok').length).toBeGreaterThan(0);
    expect(within(row).queryByText('0')).toBeNull();
  });

  it('keeps currencies apart in the totals (R253)', () => {
    const r = renderWithProviders(
      <PlantsSectionView plants={{ ...demoDashboard.plants, total_invoice: [
        { currency: 'TRY', amount: '2160' }, { currency: 'USD', amount: '10' }] }} />,
    );
    expect(r.getByText(/2.160,00 TRY/)).toBeVisible();
    expect(r.getByText(/10,00 USD/)).toBeVisible();
  });

  it("downloads the netting analyzer's bill only when there is one", async () => {
    const onDownloadPdf = vi.fn();
    const r = renderWithProviders(<PlantsSectionView plants={demoDashboard.plants} onDownloadPdf={onDownloadPdf} />);
    const buttons = r.getAllByRole('button', { name: /Faturayı indir/ });
    expect(buttons).toHaveLength(1);
    await userEvent.click(buttons[0]);
    expect(onDownloadPdf).toHaveBeenCalledWith('bill-1');
  });

  it('renders nothing for a building-scoped reader (plants are company-level)', () => {
    const r = renderWithProviders(
      <PlantsSectionView plants={{ available: false, reason: 'plants_not_in_scope', rows: [], total_invoice: [] }} />,
    );
    expect(r.queryByText('GES santralleri')).toBeNull();
    expect(r.queryByRole('table')).toBeNull();
  });

  it('says so when the company has no plants', () => {
    const r = renderWithProviders(
      <PlantsSectionView plants={{ available: true, rows: [], total_invoice: [] }} />,
    );
    expect(r.getByText(/kayıtlı santral yok/i)).toBeVisible();
  });
});
