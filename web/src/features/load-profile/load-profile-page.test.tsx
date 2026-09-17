import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import { selectionKey } from '@/lib/selection/selection-store';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { LoadProfilePage } from './load-profile-page';
import { demoProfiles, demoStatistics } from './_fixture';

const me: MeResponse = {
  id: 'u-1',
  name: 'Ayşe Kaya',
  email: 'ayse@ornek.com.tr',
  role: 'company_admin',
  locale: 'tr',
  permissions: fixture.company_admin as MeResponse['permissions'],
  session_id: 's',
  company: { id: 'c-own', name: 'Kendi Şirketim' },
};

const ROUTES = {
  'GET /api/v1/buildings': {
    items: [{ id: 'b-1', name: 'A1 Fabrika', bill_cutoff_day: 1, created_at: '', updated_at: '', activity_status: 'active' }],
  },
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
  'GET /api/v1/load-profile': { ...demoProfiles, config: { weekend_days: [5, 6], weekend_source: 'company', vacations: 1 } },
  'GET /api/v1/load-profile/statistics': demoStatistics,
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('LoadProfilePage', () => {
  it('asks for all ten profiles of the selected analyzer over the last six months', async () => {
    localStorage.setItem(selectionKey('u-1'), JSON.stringify({ buildingId: 'b-1', analyzerId: 'a-1' }));
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me}>
        <LoadProfilePage />
      </SessionProvider>,
    );
    await waitFor(() => expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/load-profile')).toBe(true));
    const query = queryOf(api.calls, 'GET', '/api/v1/load-profile');
    expect(query.get('analyzer_id')).toBe('a-1');
    expect(query.getAll('profiles')).toHaveLength(10);
    const days = (Date.parse(query.get('to')!) - Date.parse(query.get('from')!)) / 86_400_000;
    expect(days).toBe(182);
  });

  it('names the weekend days the company calendar decided', async () => {
    localStorage.setItem(selectionKey('u-1'), JSON.stringify({ buildingId: 'b-1', analyzerId: 'a-1' }));
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me}>
        <LoadProfilePage />
      </SessionProvider>,
    );
    await waitFor(() =>
      expect(r.getByText(/Hafta sonu günleri: Cuma, Cumartesi \(şirket takvimi\) · 1 tatil dönemi/)).toBeInTheDocument(),
    );
  });

  it('asks for an analyzer before requesting anything', async () => {
    api = mockApi({ ...ROUTES, 'GET /api/v1/analyzers': { items: [] } });
    const r = renderWithProviders(
      <SessionProvider me={me}>
        <LoadProfilePage />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByText('Yük profili için bir analizör seçin')).toBeInTheDocument());
    expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/load-profile')).toBe(false);
  });
});
