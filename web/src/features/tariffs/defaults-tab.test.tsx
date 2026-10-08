import { act, waitFor } from '@testing-library/react';
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
      expect.any(Function),
    );
  });

  it('reads a Turkish comma, omits a cleared optional and stays open until the save succeeds', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('button', { name: /Yeni satır/ }));
    await user.type(r.getByLabelText(/Yürürlük tarihi/), '2026-07-01');
    await user.type(r.getByLabelText(/Enerji fiyatı/), '3,4');
    await user.type(r.getByLabelText(/Dağıtım fiyatı/), '2,5');
    await user.type(r.getByLabelText(/KDV oranı/), '20');
    await user.type(r.getByLabelText(/Güç fiyatı/), '1');
    await user.clear(r.getByLabelText(/Güç fiyatı/));
    await user.click(r.getByRole('button', { name: /Kaydet/ }));

    const [body, done] = r.onPublish.mock.calls[0]!;
    expect(body).toMatchObject({ energy_price: '3.4', distribution_price: '2.5' });
    expect(body).not.toHaveProperty('power_price');
    expect(r.getByRole('dialog')).toBeVisible();
    act(() => done());
    await waitFor(() => expect(r.queryByRole('dialog')).toBeNull());
  });

  it('says which required fields are missing instead of doing nothing', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('button', { name: /Yeni satır/ }));
    await user.type(r.getByLabelText(/Enerji fiyatı/), '3,4,1');
    await user.click(r.getByRole('button', { name: /Kaydet/ }));
    expect(r.onPublish).not.toHaveBeenCalled();
    expect(r.getAllByText('Zorunlu alan')).toHaveLength(3);
    expect(r.getByText('Geçerli bir sayı girin, örn. 1234,56')).toBeVisible();
  });

  it('shows the API field errors it is given', async () => {
    const user = userEvent.setup();
    const r = view({ fieldErrors: { vat_rate: 'Aralık dışında' } });
    await user.click(r.getByRole('button', { name: /Yeni satır/ }));
    expect(r.getByText('Aralık dışında')).toBeVisible();
  });

  it('shows an empty state', () => {
    expect(view({ entries: [] }).getByText(/Yayımlanmış tarife yok/)).toBeVisible();
  });
});
