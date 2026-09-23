import { screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { demoMonthly, demoYearly } from './_fixture';
import { ReportsPage } from './reports-page';

const me = (role: keyof typeof fixture): MeResponse => ({
  id: 'u-1', name: 'Ayşe Kaya', email: 'ayse@ornek.com.tr', role, locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'], session_id: 's',
  company: { id: 'c-own', name: 'Kendi Şirketim' },
});

const building = (id: string, name: string) => ({ id, company_id: 'c-own', name, bill_cutoff_day: 1, created_at: '', updated_at: '' });

const ROUTES = {
  'GET /api/v1/buildings': { items: [building('b-1', 'Merkez'), building('b-2', 'Depo')], total: 2 },
  'GET /api/v1/power-plants': { items: [{ id: 'p-1', company_id: 'c-own', name: 'Arazi GES', plant_kind: 'grid', created_at: '', updated_at: '' }] },
  'GET /api/v1/reports/preview': (req: Request) =>
    Response.json(
      new URL(req.url).searchParams.get('type') === 'yearly'
        ? { version: 1, type: 'yearly', period: '2025', yearly: demoYearly }
        : { version: 1, type: 'monthly', period: '2026-08', monthly: demoMonthly },
    ),
  'GET /api/v1/reports': { items: [], next_cursor: null, total: 0 },
};

let api: ReturnType<typeof mockApi>;
/** The newest preview request's query. */
const lastPreview = () => {
  const n = api.calls.filter((c) => new URL(c.url).pathname === '/api/v1/reports/preview').length;
  return queryOf(api.calls, 'GET', '/api/v1/reports/preview', n - 1);
};
const render = (role: keyof typeof fixture = 'company_admin') =>
  renderWithProviders(
    <SessionProvider me={me(role)}>
      <ReportsPage />
    </SessionProvider>,
  );

beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('ReportsPage', () => {
  it('previews the first building’s previous month', async () => {
    api = mockApi(ROUTES);
    render();
    expect(await screen.findByRole('table', { name: 'Bilgi tablosu' })).toBeVisible();
    const q = queryOf(api.calls, 'GET', '/api/v1/reports/preview');
    expect(q.get('type')).toBe('monthly');
    expect(q.getAll('building_ids')).toEqual(['b-1']);
    expect(q.get('plant_selection')).toBe('all');
    expect(q.get('period')).toMatch(/^\d{4}-\d{2}$/);
  });

  it('switches to the yearly tab and asks for a year', async () => {
    api = mockApi(ROUTES);
    const { user } = render();
    await user.click(await screen.findByRole('tab', { name: 'Yıllık rapor' }));
    await waitFor(() => {
      expect(lastPreview().get('type')).toBe('yearly');
    });
    expect(lastPreview().get('period')).toMatch(/^\d{4}$/);
    expect(await screen.findByRole('table', { name: 'Karbon emisyonu' })).toBeVisible();
  });

  it('never lists plants for a building admin (GET /power-plants is A CA CR)', async () => {
    api = mockApi(ROUTES);
    render('building_admin');
    expect(await screen.findByRole('table', { name: 'Bilgi tablosu' })).toBeVisible();
    expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/power-plants')).toBe(false);
  });
});
