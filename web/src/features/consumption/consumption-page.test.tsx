import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import { selectionKey } from '@/lib/selection/selection-store';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { ConsumptionPage } from './consumption-page';

const me = (role: keyof typeof fixture = 'company_admin'): MeResponse => ({
  id: 'u-1',
  name: 'Ayşe Kaya',
  email: 'ayse@ornek.com.tr',
  role,
  locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'],
  session_id: 's',
  company: { id: 'c-own', name: 'Kendi Şirketim' },
});

const ASSETS = {
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
  'GET /api/v1/consumption': { items: [] },
  'GET /api/v1/consumption/summary': { rows: 0, suspect_rows: 0, totals: {}, averages: {} },
  'GET /api/v1/consumption/grouped': {
    group_by: 'daily',
    current: { from: '2026-03-01', to: '2026-03-31', groups: [], statistics: {} },
  },
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('ConsumptionPage', () => {
  it('queries the selected analyzer with the last six months, daily (R197)', async () => {
    localStorage.setItem(selectionKey('u-1'), JSON.stringify({ buildingId: 'b-1', analyzerId: 'a-1' }));
    api = mockApi(ASSETS);
    renderWithProviders(
      <SessionProvider me={me()}>
        <ConsumptionPage />
      </SessionProvider>,
    );
    await waitFor(() => expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/consumption')).toBe(true));
    const query = queryOf(api.calls, 'GET', '/api/v1/consumption');
    expect(query.get('analyzer_id')).toBe('a-1');
    expect(query.get('granularity')).toBe('daily');
    const days = (Date.parse(query.get('to')!) - Date.parse(query.get('from')!)) / 86_400_000;
    expect(days).toBe(182);
  });

  it('asks for the grouped data only when the detailed tab is opened', async () => {
    localStorage.setItem(selectionKey('u-1'), JSON.stringify({ buildingId: 'b-1', analyzerId: 'a-1' }));
    api = mockApi(ASSETS);
    const r = renderWithProviders(
      <SessionProvider me={me()}>
        <ConsumptionPage />
      </SessionProvider>,
    );
    await waitFor(() => expect(api.calls.length).toBeGreaterThan(2));
    expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/consumption/grouped')).toBe(false);

    await r.user.click(r.getByRole('tab', { name: 'Detaylı grafikler' }));
    await waitFor(() =>
      expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/consumption/grouped')).toBe(true),
    );
    expect(queryOf(api.calls, 'GET', '/api/v1/consumption/grouped').get('group_by')).toBe('daily');
  });

  it('asks the user to choose a subject before querying anything', async () => {
    api = mockApi({ ...ASSETS, 'GET /api/v1/buildings': { items: [] }, 'GET /api/v1/analyzers': { items: [] } });
    const r = renderWithProviders(
      <SessionProvider me={me()}>
        <ConsumptionPage />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByText('Verileri görmek için bir bina veya analizör seçin')).toBeInTheDocument());
    expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/consumption')).toBe(false);
  });
});
