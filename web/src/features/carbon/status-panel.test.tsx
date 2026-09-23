import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { activity, BUILDING, catalogue, factor, me } from './_fixture';
import { StatusPanel } from './status-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

let statusBody: unknown = null;
const routes = {
  'GET /api/v1/carbon/activities': { items: [activity()], next_cursor: null },
  'GET /api/v1/carbon/activity-catalogue': catalogue,
  'GET /api/v1/carbon/emission-factors': { items: [factor()] },
  'POST /api/v1/carbon/activities/a-1/status': (req: Request) =>
    req.json().then((b: unknown) => {
      statusBody = b;
      return Response.json(activity({ status: 'approved' }));
    }),
  'DELETE /api/v1/carbon/activities/a-1': new Response(null, { status: 204 }),
};

const render = (role: 'company_admin' | 'building_admin' = 'company_admin') =>
  renderWithProviders(
    <SessionProvider me={me(role)}>
      <StatusPanel buildingId={BUILDING} />
    </SessionProvider>,
  );

describe('StatusPanel', () => {
  it('lists the building records with the filters in the query', async () => {
    api = mockApi(routes);
    const r = render();
    expect(await r.findByText('Onay Bekliyor')).toBeVisible();
    const url = new URL(api.calls.find((c) => c.url.includes('/carbon/activities'))!.url);
    expect(url.searchParams.get('building_id')).toBe(BUILDING);
    expect(url.searchParams.get('status')).toBeNull();
  });

  it('approves through the status route', async () => {
    api = mockApi(routes);
    const r = render();
    await r.user.click(await r.findByRole('button', { name: 'Onayla' }));
    await waitFor(() => expect(statusBody).toEqual({ status: 'approved' }));
  });

  it('asks before deleting', async () => {
    api = mockApi(routes);
    const r = render();
    await r.user.click(await r.findByRole('button', { name: 'Sil' }));
    expect(await r.findByRole('dialog', { name: 'Kaydı sil' })).toBeVisible();
    expect(api.calls.some((c) => c.method === 'DELETE')).toBe(false);
    await r.user.click(r.getByRole('button', { name: 'Sil' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'DELETE')).toBe(true));
  });

  it('opens the edit dialog prefilled', async () => {
    api = mockApi(routes);
    const r = render();
    await r.user.click(await r.findByRole('button', { name: 'Düzenle' }));
    expect(await r.findByRole('dialog', { name: 'Ortam Isıtması kaydını düzenle' })).toBeVisible();
  });

  it('shows a building admin no actions', async () => {
    api = mockApi(routes);
    const r = render('building_admin');
    expect(await r.findByText('Onay Bekliyor')).toBeVisible();
    expect(r.queryByRole('button', { name: 'Onayla' })).toBeNull();
  });
});
