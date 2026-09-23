import { screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { demoArchive, demoMonthly, demoYearly } from './_fixture';
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
  'GET /api/v1/reports': (req: Request) =>
    Response.json(
      new URL(req.url).searchParams.get('type') === 'yearly'
        ? { items: [], next_cursor: null, total: 0 }
        : { items: demoArchive, next_cursor: null, total: demoArchive.length },
    ),
  'POST /api/v1/reports/generate': Response.json(
    { items: [{ building_id: 'b-1', report_id: 'r-9', job_id: 'report.generate:b-1:monthly:2026-08' }] },
    { status: 202 },
  ),
  // openapi-fetch percent-encodes the colons in the deterministic task id.
  'GET /api/v1/jobs/report.generate%3Ab-1%3Amonthly%3A2026-08': { id: 'report.generate:b-1:monthly:2026-08', type: 'report.generate', status: 'succeeded' },
  'POST /api/v1/reports/r-9/email': Response.json({ job_id: 'report.deliver:r-9:x' }, { status: 202 }),
  'GET /api/v1/jobs/report.deliver%3Ar-9%3Ax': { id: 'report.deliver:r-9:x', type: 'report.deliver', status: 'failed', error_code: 'smtp_not_configured' },
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

  it('generates per building, then offers the files and e-mail of the ready report', async () => {
    api = mockApi(ROUTES);
    const { user } = render();
    await screen.findByRole('table', { name: 'Bilgi tablosu' });
    await user.click(screen.getByRole('button', { name: 'Raporu oluştur' }));
    const generate = api.calls.find((c) => c.method === 'POST' && new URL(c.url).pathname === '/api/v1/reports/generate');
    const body = await generate!.clone().json();
    expect(body).toMatchObject({ type: 'monthly', plant_selection: 'all', building_ids: ['b-1'] });
    expect(await screen.findByRole('button', { name: 'PDF indir' })).toBeVisible();

    await user.click(screen.getByRole('button', { name: 'E-posta ile gönder' }));
    await user.type(screen.getByRole('textbox', { name: /Alıcı e-posta adresi/ }), 'yonetici@firma.com.tr');
    await user.click(screen.getByRole('button', { name: 'Gönder' }));
    const sent = api.calls.find((c) => c.method === 'POST' && new URL(c.url).pathname === '/api/v1/reports/r-9/email');
    expect(await sent!.clone().json()).toEqual({ to: ['yonetici@firma.com.tr'] });
    expect(await screen.findByText(/SMTP ayarları yok/)).toBeVisible();
  });

  it('shows no generation to a read-only admin', async () => {
    api = mockApi(ROUTES);
    render('company_readonly_admin');
    await screen.findByRole('table', { name: 'Bilgi tablosu' });
    expect(screen.queryByRole('button', { name: 'Raporu oluştur' })).toBeNull();
  });

  it('lists the archive with its count', async () => {
    api = mockApi(ROUTES);
    const { user } = render();
    await user.click(await screen.findByRole('tab', { name: 'Rapor arşivi' }));
    expect(await screen.findByText('Toplam rapor: 4')).toBeVisible();
    expect(screen.getByRole('heading', { name: '2025 raporları' })).toBeVisible();
  });

  it('never lists plants for a building admin (GET /power-plants is A CA CR)', async () => {
    api = mockApi(ROUTES);
    render('building_admin');
    expect(await screen.findByRole('table', { name: 'Bilgi tablosu' })).toBeVisible();
    expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/power-plants')).toBe(false);
  });
});
