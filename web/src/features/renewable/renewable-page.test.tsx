import { waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import { selectionKey } from '@/lib/selection/selection-store';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import {
  analytics,
  balance,
  efficiency,
  environmental,
  forecast,
  grid,
  overview,
  realtime,
  systemStatus,
} from './_fixture';
import { RenewablePage } from './renewable-page';

const me = (role: keyof typeof fixture = 'building_admin'): MeResponse => ({
  id: 'u-1',
  name: 'Ayşe Kaya',
  email: 'ayse@ornek.com.tr',
  role,
  locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'],
  session_id: 's',
  company: { id: 'c-own', name: 'Kendi Şirketim' },
});

const API = {
  'GET /api/v1/buildings': {
    items: [
      {
        id: 'b-1',
        name: 'A1 Fabrika',
        bill_cutoff_day: 1,
        created_at: '',
        updated_at: '',
        activity_status: 'active',
      },
    ],
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
  'GET /api/v1/renewable/overview': overview,
  'GET /api/v1/renewable/realtime': realtime,
  'GET /api/v1/renewable/grid-interaction': grid,
  'GET /api/v1/renewable/environmental': environmental,
  'GET /api/v1/renewable/efficiency': efficiency,
  'GET /api/v1/renewable/forecast': forecast,
  'GET /api/v1/renewable/analytics': analytics,
  'GET /api/v1/renewable/system-status': systemStatus,
  'GET /api/v1/generation': { items: [] },
  'GET /api/v1/energy-balance': balance,
  'GET /api/v1/weather': { available: false, reason: 'weather_not_configured', days: [] },
};

const paths = (calls: { url: string }[]) => calls.map((c) => new URL(c.url).pathname);

let api: ReturnType<typeof mockApi>;
beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('RenewablePage', () => {
  it('asks for a subject before querying anything', async () => {
    api = mockApi(API);
    const r = renderWithProviders(
      <SessionProvider me={me()}>
        <RenewablePage />
      </SessionProvider>,
    );
    expect(await r.findByText('Bir bina ya da analizör seçin')).toBeVisible();
    expect(paths(api.calls).some((p) => p.startsWith('/api/v1/renewable'))).toBe(false);
  });

  it('reads the selected analyzer: summary and the production tab (§7.8)', async () => {
    localStorage.setItem(
      selectionKey('u-1'),
      JSON.stringify({ buildingId: 'b-1', analyzerId: 'a-1' }),
    );
    api = mockApi(API);
    const r = renderWithProviders(
      <SessionProvider me={me()}>
        <RenewablePage />
      </SessionProvider>,
    );
    expect(await r.findByText('1.250,5')).toBeVisible();
    const q = queryOf(api.calls, 'GET', '/api/v1/renewable/overview');
    expect(q.get('analyzer_id')).toBe('a-1');
    expect(q.get('building_id')).toBeNull();
    await waitFor(() => expect(paths(api.calls)).toContain('/api/v1/generation'));
    expect(paths(api.calls)).not.toContain('/api/v1/renewable/realtime');
  });

  it('the detailed tab loads every panel, the energy balance and the building weather', async () => {
    localStorage.setItem(
      selectionKey('u-1'),
      JSON.stringify({ buildingId: 'b-1', analyzerId: 'a-1' }),
    );
    api = mockApi(API);
    const r = renderWithProviders(
      <SessionProvider me={me()}>
        <RenewablePage />
      </SessionProvider>,
    );
    await userEvent.click(await r.findByRole('tab', { name: 'Detaylı analiz' }));
    await waitFor(() => {
      for (const p of [
        'realtime',
        'grid-interaction',
        'environmental',
        'efficiency',
        'forecast',
        'analytics',
        'system-status',
      ]) {
        expect(paths(api.calls)).toContain(`/api/v1/renewable/${p}`);
      }
      expect(paths(api.calls)).toContain('/api/v1/energy-balance');
    });
    expect(queryOf(api.calls, 'GET', '/api/v1/weather').get('building_id')).toBe('b-1');
    expect(
      await r.findByText('Batarya filtresi kullanılamıyor: batarya ölçümü yok.'),
    ).toBeVisible();
    expect(r.getAllByText('Gerilim ve frekans ölçülmüyor.').length).toBe(3); // voltage, frequency, grid efficiency
  });
});
