import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import { selectionKey } from '@/lib/selection/selection-store';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { DashboardPage } from './dashboard-page';

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

const building = (id: string, name: string) => ({
  id,
  name,
  bill_cutoff_day: 1,
  created_at: '',
  updated_at: '',
  activity_status: 'active' as const,
  analyzer_count: 1,
  latitude: '39.92',
  longitude: '32.86',
});

const ROUTES = {
  'GET /api/v1/buildings': { items: [building('b-1', 'A1 Fabrika'), building('b-2', 'A2 Depo')] },
  'GET /api/v1/analyzers': {
    items: [
      {
        id: 'a-1',
        building_id: 'b-1',
        installation_number: '1001',
        provider: 'osos',
        provider_subtype: 'Baskent',
        meter_multiplier: '1',
        is_active: true,
        activity_status: 'active',
      },
    ],
  },
  'GET /api/v1/consumption': { items: [] },
  'GET /api/v1/consumption/reactive-status': { month: '2026-09', analyzers: [] },
  'GET /api/v1/bills/latest': () => Response.json({ error: { code: 'not_found', message: 'yok' } }, { status: 404 }),
  'GET /api/v1/buildings/b-1/comparison': { available: false, sector: 'Üretim', peers: 2, reason: 'sector_too_small' },
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('DashboardPage', () => {
  it('sends the admin company scope on every panel query', async () => {
    localStorage.setItem(selectionKey('u-admin'), JSON.stringify({ companyId: 'c-other', buildingId: 'b-1' }));
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('admin')}>
        <DashboardPage />
      </SessionProvider>,
    );
    await waitFor(() => expect(api.calls.length).toBeGreaterThan(3));
    for (const path of ['/api/v1/buildings', '/api/v1/analyzers', '/api/v1/consumption/reactive-status']) {
      expect(queryOf(api.calls, 'GET', path).get('company_id'), path).toBe('c-other');
    }
  });

  it('shows the empty bill card and the too-small sector without failing', async () => {
    localStorage.setItem(selectionKey('u-company_admin'), JSON.stringify({ buildingId: 'b-1' }));
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <DashboardPage />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByText('Henüz hesaplanmış fatura yok')).toBeInTheDocument());
    await waitFor(() =>
      expect(r.getByText('Sektörde karşılaştırma için yeterli bina yok (en az 3).')).toBeInTheDocument(),
    );
  });

  it('hides the refresh actions from a read-only role', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_readonly_admin')}>
        <DashboardPage />
      </SessionProvider>,
    );
    await waitFor(() => expect(api.calls.length).toBeGreaterThan(1));
    expect(r.queryByRole('button', { name: 'Saatlik değerleri yenile' })).toBeNull();
  });
});
