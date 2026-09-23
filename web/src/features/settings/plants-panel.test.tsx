import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { PlantsPanel } from './plants-panel';

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

const plant = { id: 'p-1', name: 'Çatı GES', plant_kind: 'rooftop', total_capacity_kw: '250.00', created_at: '' };

const ROUTES = {
  'GET /api/v1/power-plants': { items: [plant] },
  'GET /api/v1/power-plants/p-1': {
    ...plant,
    monthly_targets: Array.from({ length: 12 }, () => '1000'),
    alarm_recipients: [],
    devices: [],
  },
  'POST /api/v1/power-plants': Response.json({ id: 'p-2' }, { status: 201 }),
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('PlantsPanel', () => {
  it('cannot save a new plant until every month has a target', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <PlantsPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByRole('button', { name: 'Santral ekle' })).toBeInTheDocument());
    await r.user.click(r.getByRole('button', { name: 'Santral ekle' }));
    expect(await r.findByText('12 ayın hedefi girilmeli')).toBeInTheDocument();
    expect(r.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
    expect(api.calls.some((c) => c.method === 'POST')).toBe(false);
  });

  it('shows a read-only role the plants without any control', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_readonly_admin')}>
        <PlantsPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByRole('row', { name: /Çatı GES/ })).toBeInTheDocument());
    expect(r.queryByRole('button', { name: 'Santral ekle' })).toBeNull();
  });
});

describe('PlantsPanel iSolar link (R281)', () => {
  const linkRoutes = {
    ...ROUTES,
    'GET /api/v1/integration-credentials': { items: [
      { id: 'c-off', provider: 'isolar', subtype: 'CN', definition_id: 'd', has_secret: true, extra_keys: [], is_active: false, updated_at: '' },
      { id: 'c-iso', provider: 'isolar', subtype: 'EU', definition_id: 'd', has_secret: true, extra_keys: [], is_active: true, updated_at: '' },
      { id: 'c-osos', provider: 'osos', subtype: 'Baskent', definition_id: 'd2', has_secret: true, extra_keys: [], is_active: true, updated_at: '' },
    ] },
    'GET /api/v1/analyzers': { items: [], next_cursor: null },
    // A finished job stops the poll, so nothing outlives the test.
    'GET /api/v1/jobs/isolar.sync_plant%3Ap-1%3A400': { id: 'isolar.sync_plant:p-1:400', type: 'isolar.sync_plant', status: 'succeeded' },
    'GET /api/v1/integrations/isolar/plants': { items: [{ ps_id: 'PS-1', name: 'Konya Solar', installed_kw: '250' }] },
    'POST /api/v1/plants/p-1/isolar-link': (request: Request) =>
      request.json().then((body: { credential_id?: string; ps_id?: string }) =>
        body.credential_id === 'c-iso' && body.ps_id === 'PS-1'
          ? Response.json({ plant: { ...plant, monthly_targets: [], alarm_recipients: [], devices: [] }, job_id: 'isolar.sync_plant:p-1:400' })
          : Response.json({ error: { code: 'validation_failed' } }, { status: 422 })),
  };

  it('links a plant through the account list and only offers active iSolar credentials', async () => {
    api = mockApi(linkRoutes);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <PlantsPanel />
      </SessionProvider>,
    );
    await r.user.click(await r.findByRole('button', { name: 'iSolarCloud’a bağla' }));
    await r.user.click(await r.findByRole('radio', { name: /Konya Solar/ }));
    await r.user.click(r.getByRole('button', { name: 'Bağla' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'POST' && c.url.includes('/isolar-link'))).toBe(true));
    const accountCall = api.calls.find((c) => c.url.includes('/integrations/isolar/plants'));
    expect(new URL(accountCall!.url).searchParams.get('credential_id')).toBe('c-iso');
  });
});
