import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { CalendarPage } from './calendar-page';
import { demoEvents, demoPeriods, demoWeekendDays } from './_fixture';

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
  'GET /api/v1/calendar/events': { items: demoEvents },
  'GET /api/v1/calendar/vacations': { weekend_days: demoWeekendDays, weekend_source: 'company', periods: demoPeriods },
  'POST /api/v1/calendar/events': Response.json({ id: 'e-9' }, { status: 201 }),
  'PUT /api/v1/calendar/vacations': new Response(null, { status: 204 }),
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => {
  localStorage.clear();
  vi.setSystemTime(new Date('2026-03-14T09:00:00+03:00'));
});
afterEach(() => {
  api.restore();
  vi.useRealTimers();
});

describe('CalendarPage', () => {
  it('asks for the visible range and moves it with the toolbar', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <CalendarPage />
      </SessionProvider>,
    );
    await waitFor(() => expect(api.calls.some((c) => c.url.includes('/calendar/events'))).toBe(true));
    expect(queryOf(api.calls, 'GET', '/api/v1/calendar/events').get('from')).toBe('2026-02-23');

    await r.user.click(r.getByRole('button', { name: 'Sonraki' }));
    await waitFor(() => expect(queryOf(api.calls, 'GET', '/api/v1/calendar/events', 1).get('from')).toBe('2026-03-30'));
  });

  it('creates an event from the toolbar', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <CalendarPage />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getAllByRole('button', { name: 'Etkinlik ekle' }).length).toBeGreaterThan(0));
    await r.user.click(r.getAllByRole('button', { name: 'Etkinlik ekle' })[0]);
    await r.user.type(await r.findByLabelText(/Başlık/), 'Yeni etkinlik');
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'POST')).toBe(true));
    const body = await api.calls.find((c) => c.method === 'POST')!.clone().json();
    expect(body).toMatchObject({ title: 'Yeni etkinlik', all_day: true, starts_at: '2026-03-14T00:00:00+03:00' });
  });

  it('lets a read-only role look but not touch', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('building_readonly_admin')}>
        <CalendarPage />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getAllByRole('button', { name: 'Bakım günü' }).length).toBeGreaterThan(0));
    expect(r.queryByRole('button', { name: 'Etkinlik ekle' })).toBeNull();
    await r.user.click(r.getByRole('button', { name: 'Tatil yönetimi' }));
    expect(await r.findByRole('dialog')).toBeInTheDocument();
    expect(r.queryByRole('button', { name: 'Kaydet' })).toBeNull();
  });
});
