import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

const searchParams = vi.hoisted(() => ({ current: new URLSearchParams() }));
vi.mock('next/navigation', () => ({ useSearchParams: () => searchParams.current }));

const { AnalyzersPanel } = await import('./analyzers-panel');

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

const ROUTES = {
  'GET /api/v1/analyzers': {
    items: [
      {
        id: 'a-1',
        building_id: 'b-1',
        installation_number: '4001234567',
        provider: 'osos',
        provider_subtype: 'Baskent',
        meter_multiplier: '1',
        is_active: true,
        activity_status: 'active',
      },
    ],
  },
  'GET /api/v1/buildings': { items: [{ id: 'b-1', name: 'A1 Fabrika', bill_cutoff_day: 1, created_at: '', updated_at: '' }] },
  'PATCH /api/v1/analyzers/a-1': Response.json({ id: 'a-1' }),
  'POST /api/v1/analyzers/a-1/refresh': Response.json({ job_id: 'job-9' }, { status: 202 }),
  'GET /api/v1/jobs/job-9': { id: 'job-9', type: 'integration.refresh_analyzer', status: 'queued' },
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => {
  localStorage.clear();
  searchParams.current = new URLSearchParams();
});
afterEach(() => api.restore());

describe('AnalyzersPanel', () => {
  it('starts on the unassigned filter when discovery sent the user here', async () => {
    searchParams.current = new URLSearchParams('unassigned=1');
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <AnalyzersPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/analyzers')).toBe(true));
    expect(queryOf(api.calls, 'GET', '/api/v1/analyzers').get('unassigned')).toBe('true');
  });

  it('assigns an analyzer to a building', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <AnalyzersPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByRole('button', { name: /İşlemler/ })).toBeInTheDocument());
    await r.user.click(r.getByRole('button', { name: /İşlemler/ }));
    await r.user.click(await r.findByRole('menuitem', { name: 'Binaya ata' }));
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'PATCH')).toBe(true));
    expect(await api.calls.find((c) => c.method === 'PATCH')!.clone().json()).toEqual({ building_id: 'b-1' });
  });

  it('polls the refresh job it started', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('building_admin')}>
        <AnalyzersPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByRole('button', { name: /İşlemler/ })).toBeInTheDocument());
    await r.user.click(r.getByRole('button', { name: /İşlemler/ }));
    await r.user.click(await r.findByRole('menuitem', { name: 'Saatlik değerleri yenile' }));
    await waitFor(() => expect(api.calls.some((c) => c.url.includes('/jobs/job-9'))).toBe(true));
    expect(r.getByText(/Saatlik değerleri yenile: Sırada/)).toBeInTheDocument();
  });
});
