import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { BuildingsPanel } from './buildings-panel';

const me = (role: keyof typeof fixture): MeResponse => ({
  id: `u-${role}`,
  name: 'Ayşe Kaya',
  email: 'ayse@ornek.com.tr',
  role,
  locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'],
  session_id: 's',
  company: { id: 'c-own', name: 'Kendi Şirketim' },
});

const building = {
  id: 'b-1',
  name: 'A1 Fabrika',
  bill_cutoff_day: 5,
  analyzer_count: 2,
  activity_status: 'active',
  created_at: '',
  updated_at: '',
};

const ROUTES = {
  'GET /api/v1/buildings': { items: [building] },
  'GET /api/v1/buildings/b-1': { ...building, contacts: [], tariff_history: [] },
  'GET /api/v1/users': { items: [] },
  'POST /api/v1/buildings': Response.json({ id: 'b-2' }, { status: 201 }),
  'DELETE /api/v1/buildings/b-1': new Response(null, { status: 204 }),
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('BuildingsPanel', () => {
  it('creates a building through the API', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <BuildingsPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByRole('button', { name: 'Bina ekle' })).toBeInTheDocument());
    await r.user.click(r.getByRole('button', { name: 'Bina ekle' }));
    await r.user.type(await r.findByLabelText(/Bina adı/), 'Yeni Bina');
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'POST')).toBe(true));
    const body = await api.calls.find((c) => c.method === 'POST')!.clone().json();
    expect(body).toMatchObject({ name: 'Yeni Bina', bill_cutoff_day: 1, clear_responsible_user: true });
  });

  it('asks before deleting and reports the API message on refusal', async () => {
    api = mockApi({
      ...ROUTES,
      'DELETE /api/v1/buildings/b-1': Response.json(
        { error: { code: 'building_has_analyzers', message: 'Binaya bağlı analizörler var.' } },
        { status: 409 },
      ),
    });
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <BuildingsPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByRole('button', { name: 'Binayı sil' })).toBeInTheDocument());
    await r.user.click(r.getByRole('button', { name: 'Binayı sil' }));
    expect(await r.findByText('A1 Fabrika binası silinsin mi?')).toBeInTheDocument();
    await r.user.click(r.getByRole('button', { name: 'Sil' }));
    await waitFor(() => expect(r.getByText('Binaya bağlı analizörler var.')).toBeInTheDocument());
  });

  it('gives a read-only role the list without any control, and asks for no users', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_readonly_admin')}>
        <BuildingsPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByRole('row', { name: /A1 Fabrika/ })).toBeInTheDocument());
    expect(r.queryByRole('button', { name: 'Bina ekle' })).toBeNull();
    expect(queryOf(api.calls, 'GET', '/api/v1/users').toString()).toBe('');
  });
});
