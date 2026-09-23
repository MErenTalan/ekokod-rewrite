import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';

import { renderWithProviders } from '@/test/render';

import { demoTariffSummaries } from './_fixture';
import { TariffHistoryView } from './tariff-history';

describe('TariffHistoryView', () => {
  it('lists every version with its effective date and pricing mode', () => {
    const r = renderWithProviders(<TariffHistoryView tariffs={demoTariffSummaries} canEdit />);
    // The house date formatter (lib/format) renders the medium form, the same
    // one the alarm log and Messages use; the screens do not invent a second.
    expect(r.getByRole('cell', { name: /Tem 2026/ })).toBeVisible();
    expect(r.getByRole('cell', { name: 'PTF geçişi' })).toBeVisible();
    expect(r.getByText('PTF + YEKDEM')).toBeVisible();
    expect(r.getByText('Sabit fiyat')).toBeVisible();
  });

  it('says plainly that issued invoices keep their own version', () => {
    const r = renderWithProviders(<TariffHistoryView tariffs={demoTariffSummaries} canEdit />);
    expect(r.getByText(/kesildikleri andaki tarife sürümünü/i)).toBeVisible();
  });

  it('offers edit and delete only to a principal who may edit', () => {
    const readOnly = renderWithProviders(<TariffHistoryView tariffs={demoTariffSummaries} canEdit={false} />);
    expect(readOnly.queryByRole('button', { name: /Düzenle/ })).toBeNull();
    expect(readOnly.queryByRole('button', { name: /Sil/ })).toBeNull();
  });

  it('asks the row it was told to edit', async () => {
    const user = userEvent.setup();
    const onEdit = vi.fn();
    const r = renderWithProviders(<TariffHistoryView tariffs={demoTariffSummaries} canEdit onEdit={onEdit} />);
    await user.click(r.getAllByRole('button', { name: /Düzenle/ })[0]);
    expect(onEdit).toHaveBeenCalledWith('t-2');
  });

  it('shows an empty state when the building has no tariff', () => {
    const r = renderWithProviders(<TariffHistoryView tariffs={[]} canEdit />);
    expect(r.getByText(/henüz tarifesi yok/i)).toBeVisible();
  });
});
