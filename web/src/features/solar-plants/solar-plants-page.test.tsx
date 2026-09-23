import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { demoAlarms, demoDaily, demoDevices, demoMonthly, demoPlants, demoRealtime, demoRevenue } from './_fixture';
import { SolarPlantsPage } from './solar-plants-page';

const me = (role: keyof typeof fixture): MeResponse => ({
  id: 'u-1', name: 'Ayşe Kaya', email: 'ayse@ornek.com.tr', role, locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'], session_id: 's',
  company: { id: 'c-own', name: 'Kendi Şirketim' },
});

// Mirrors R284: the mock refuses what the API refuses.
const production = (request: Request) => {
  const q = new URL(request.url).searchParams;
  const days = (Date.parse(q.get('to') ?? '') - Date.parse(q.get('from') ?? '')) / 86_400_000;
  if (q.get('granularity') === 'hour' && days > 31) return Response.json({ error: { code: 'validation_failed' } }, { status: 422 });
  return Response.json(q.get('granularity') === 'month' ? demoMonthly : demoDaily);
};

const routes = (plants = demoPlants) => ({
  'GET /api/v1/power-plants': { items: plants, next_cursor: null },
  'GET /api/v1/plants/p-1/realtime': demoRealtime,
  'GET /api/v1/plants/p-1/revenue': demoRevenue,
  'GET /api/v1/plants/p-1/production': production,
  'GET /api/v1/plants/p-1/devices': { items: demoDevices },
  'GET /api/v1/plants/p-1/alarms': demoAlarms,
  'GET /api/v1/weather': { available: false, reason: 'location_not_configured', days: [] },
  'POST /api/v1/plants/p-1/sync': Response.json({ job_id: 'isolar.sync_plant:p-1:0' }, { status: 202 }),
  'GET /api/v1/jobs/isolar.sync_plant%3Ap-1%3A0': { id: 'isolar.sync_plant:p-1:0', type: 'isolar.sync_plant', status: 'succeeded' },
});

let api: ReturnType<typeof mockApi>;
const render = (role: keyof typeof fixture = 'company_admin') =>
  renderWithProviders(
    <SessionProvider me={me(role)}>
      <SolarPlantsPage />
    </SessionProvider>,
  );

beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('SolarPlantsPage', () => {
  it('explains how to link a plant when none is linked', async () => {
    api = mockApi(routes([demoPlants[2]]));
    render();
    expect(await screen.findByText(/iSolarCloud'a bağlı santral yok/)).toBeVisible();
    expect(screen.getByRole('link', { name: /Ayarlar/ })).toHaveAttribute('href', '/ekorm/settings?tab=plants');
  });

  it('opens the first linked plant with its connection state and figures', async () => {
    api = mockApi(routes());
    render();
    expect(await screen.findByText('125,4')).toBeVisible();
    expect(screen.getByText('Bağlı')).toBeVisible();
    expect(screen.getByRole('combobox', { name: /Santral/ })).toHaveTextContent('Konya GES');
  });

  it('starts a sync and watches it (R288)', async () => {
    const user = userEvent.setup();
    api = mockApi(routes());
    render();
    await user.click(await screen.findByRole('button', { name: /Verileri güncelle/ }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'POST' && c.url.includes('/plants/p-1/sync'))).toBe(true));
    expect((await screen.findAllByText(/Santral verileri/)).length).toBeGreaterThan(0);
  });

  it('a failed sync names its reason, not only "Başarısız" (R288)', async () => {
    const user = userEvent.setup();
    api = mockApi({
      ...routes(),
      'GET /api/v1/jobs/isolar.sync_plant%3Ap-1%3A0': { id: 'isolar.sync_plant:p-1:0', type: 'isolar.sync_plant', status: 'failed', error_code: 'credential_missing' },
    });
    render();
    await user.click(await screen.findByRole('button', { name: /Verileri güncelle/ }));
    expect(await screen.findByText(/Santralin iSolarCloud kimlik bilgisi yok/)).toBeVisible();
  });

  it('offers no sync to a read-only admin', async () => {
    api = mockApi(routes());
    render('company_readonly_admin');
    expect(await screen.findByText('125,4')).toBeVisible();
    expect(screen.queryByRole('button', { name: /Verileri güncelle/ })).toBeNull();
  });
});
