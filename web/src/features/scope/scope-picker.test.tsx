import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import { selectionKey, useSelection } from '@/lib/selection/selection-store';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { ScopePicker } from './scope-picker';

const me = (role: keyof typeof fixture = 'company_admin'): MeResponse => ({
  id: `user-${role}`,
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
  created_at: '2026-01-01T00:00:00+03:00',
  updated_at: '2026-01-01T00:00:00+03:00',
  activity_status: 'active' as const,
});
const analyzer = (id: string, buildingId: string, installation: string) => ({
  id,
  building_id: buildingId,
  installation_number: installation,
  provider: 'osos' as const,
  provider_subtype: 'Baskent',
  meter_multiplier: '1',
  is_active: true,
  activity_status: 'active' as const,
});

const ASSETS = {
  'GET /api/v1/buildings': { items: [building('b-2', 'B Depo'), building('b-1', 'A Fabrika')] },
  'GET /api/v1/analyzers': {
    items: [analyzer('a-1', 'b-1', '1001'), analyzer('a-2', 'b-2', '2001'), analyzer('a-3', 'b-2', '2002')],
  },
};

function Probe() {
  const s = useSelection();
  return <output data-testid="selection">{JSON.stringify({ b: s.buildingId, a: s.analyzerId })}</output>;
}

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());
beforeEach(() => localStorage.clear());

describe('ScopePicker', () => {
  it('selects the first building by Turkish name and its only analyzer', async () => {
    api = mockApi(ASSETS);
    const r = renderWithProviders(
      <SessionProvider me={me()}>
        <ScopePicker activeOnly={false} onActiveOnlyChange={() => {}} />
        <Probe />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByTestId('selection')).toHaveTextContent('{"b":"b-1","a":"a-1"}'));
  });

  it('keeps a stored building but drops an analyzer that is not visible', async () => {
    localStorage.setItem(selectionKey('user-company_admin'), JSON.stringify({ buildingId: 'b-2', analyzerId: 'gone' }));
    api = mockApi(ASSETS);
    const r = renderWithProviders(
      <SessionProvider me={me()}>
        <ScopePicker activeOnly={false} onActiveOnlyChange={() => {}} />
        <Probe />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByTestId('selection')).toHaveTextContent('{"b":"b-2"}'));
  });

  it('sends the admin company scope on both asset queries', async () => {
    localStorage.setItem(selectionKey('user-admin'), JSON.stringify({ companyId: 'c-other' }));
    api = mockApi(ASSETS);
    renderWithProviders(
      <SessionProvider me={me('admin')}>
        <ScopePicker activeOnly={false} onActiveOnlyChange={() => {}} />
      </SessionProvider>,
    );
    await waitFor(() => expect(api.calls.length).toBeGreaterThanOrEqual(2));
    expect(queryOf(api.calls, 'GET', '/api/v1/buildings').get('company_id')).toBe('c-other');
    expect(queryOf(api.calls, 'GET', '/api/v1/analyzers').get('company_id')).toBe('c-other');
    expect(queryOf(api.calls, 'GET', '/api/v1/buildings').getAll('include')).toEqual(['analyzer_count', 'active_status']);
  });
});
