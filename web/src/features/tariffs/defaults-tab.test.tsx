import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';

import { renderWithProviders } from '@/test/render';

import { demoNationalTariffs } from './_fixture';
import { DefaultsTabView } from './defaults-tab';

const view = (over: Partial<Parameters<typeof DefaultsTabView>[0]> = {}) => {
  const onPublish = vi.fn();
  const r = renderWithProviders(
    <DefaultsTabView entries={demoNationalTariffs} onPublish={onPublish} onDelete={() => {}} {...over} />,
  );
  return { ...r, onPublish };
};

describe('DefaultsTabView', () => {
  it('lists the published catalogue rows', () => {
    const r = view();
    expect(r.getByRole('cell', { name: 'Ticarethane' })).toBeVisible();
    expect(r.getByRole('cell', { name: 'EPDK' })).toBeVisible();
  });

  it('says plainly that publishing the same key edits the row', () => {
    expect(view().getByText(/aynı tarih, abone grubu/i)).toBeVisible();
  });

  it('publishes a new row', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('button', { name: /Yeni satır/ }));
    await user.type(r.getByLabelText(/Yürürlük tarihi/), '2026-07-01');
    await user.type(r.getByLabelText(/Enerji fiyatı/), '3.4');
    await user.type(r.getByLabelText(/Dağıtım fiyatı/), '2.5');
    await user.type(r.getByLabelText(/KDV oranı/), '20');
    await user.click(r.getByRole('button', { name: /Kaydet/ }));

    expect(r.onPublish).toHaveBeenCalledWith(
      expect.objectContaining({ effective_from: '2026-07-01', energy_price: '3.4', distribution_price: '2.5', vat_rate: '20' }),
    );
  });

  it('shows an empty state', () => {
    expect(view({ entries: [] }).getByText(/Yayımlanmış tarife yok/)).toBeVisible();
  });
});
