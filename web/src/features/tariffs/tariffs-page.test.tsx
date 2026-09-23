import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import { selectionKey } from '@/lib/selection/selection-store';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { demoTariff, demoTariffSummaries } from './_fixture';
import { TariffsPage } from './tariffs-page';

const me = (role: keyof typeof fixture): MeResponse => ({
  id: 'u-1', name: 'Ayşe Kaya', email: 'ayse@ornek.com.tr', role, locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'], session_id: 's',
  company: { id: 'c-own', name: 'Kendi Şirketim' },
});

const ROUTES = {
  'GET /api/v1/tariffs': { items: demoTariffSummaries, total: 2 },
  'GET /api/v1/tariffs/t-2': demoTariff,
  'POST /api/v1/tariffs': Response.json({ ...demoTariff, id: 't-9' }, { status: 201 }),
  'PATCH /api/v1/tariffs/t-2': Response.json(demoTariff),
  'DELETE /api/v1/tariffs/t-2': new Response(null, { status: 204 }),
};

let api: ReturnType<typeof mockApi>;
const selectBuilding = () => localStorage.setItem(selectionKey('u-1'), JSON.stringify({ buildingId: 'b-1' }));
const render = (role: keyof typeof fixture = 'company_admin') =>
  renderWithProviders(
    <SessionProvider me={me(role)}>
      <TariffsPage />
    </SessionProvider>,
  );

beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('TariffsPage', () => {
  it('asks for a building before it shows any tariff history', async () => {
    api = mockApi(ROUTES);
    render();
    expect(await screen.findByText(/bir bina seçin/i)).toBeVisible();
    expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/tariffs')).toBe(false);
  });

  it('lists the selected building’s versions', async () => {
    selectBuilding();
    api = mockApi(ROUTES);
    render();
    expect(await screen.findByRole('cell', { name: 'PTF geçişi' })).toBeVisible();
    expect(screen.getByRole('cell', { name: 'Kuruluş tarifesi' })).toBeVisible();
  });

  it('offers no write action to a read-only admin', async () => {
    selectBuilding();
    api = mockApi(ROUTES);
    render('company_readonly_admin');
    expect(await screen.findByRole('cell', { name: 'PTF geçişi' })).toBeVisible();
    expect(screen.queryByRole('button', { name: /Yeni tarife/ })).toBeNull();
    expect(screen.queryByRole('button', { name: /Düzenle/ })).toBeNull();
  });

  it('creates a version for the selected building', async () => {
    selectBuilding();
    const user = userEvent.setup();
    api = mockApi(ROUTES);
    render();

    await user.click(await screen.findByRole('button', { name: /Yeni tarife/ }));
    await user.type(screen.getByLabelText(/Yürürlük tarihi/), '2026-10-01');
    await user.type(screen.getByLabelText(/Tek zamanlı fiyat/), '3.15');
    await user.type(screen.getByLabelText(/Dağıtım bedeli/), '0.85');
    await user.type(screen.getByLabelText(/Reaktif güç bedeli/), '1.2');
    await user.type(screen.getByLabelText(/KDV oranı/), '20');
    await user.click(screen.getByRole('button', { name: /Kaydet/ }));

    await waitFor(() => {
      const post = api.calls.find((c) => c.method === 'POST' && new URL(c.url).pathname === '/api/v1/tariffs');
      expect(post).toBeDefined();
    });
  });

  it('opens an existing version in the form', async () => {
    selectBuilding();
    const user = userEvent.setup();
    api = mockApi(ROUTES);
    render();

    await user.click((await screen.findAllByRole('button', { name: /Düzenle/ }))[0]);
    expect(await screen.findByLabelText(/Enerji KBK/)).toHaveValue('1.080000');
  });
});
