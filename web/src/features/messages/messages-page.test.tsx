import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { demoMessages, demoRuns } from './_fixture';
import { MessagesPage } from './messages-page';

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
  'GET /api/v1/messages': { items: demoMessages, next_cursor: null },
  'GET /api/v1/job-runs': { items: demoRuns, next_cursor: null },
  'POST /api/v1/job-runs/alarm.evaluate/trigger': Response.json({ job_id: 'task-1' }, { status: 202 }),
  'GET /api/v1/jobs/task-1': { id: 'task-1', type: 'alarm.evaluate', status: 'succeeded' },
};

const render = (role: keyof typeof fixture) =>
  renderWithProviders(
    <SessionProvider me={me(role)}>
      <MessagesPage />
    </SessionProvider>,
  );

let api: ReturnType<typeof mockApi>;
beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('MessagesPage', () => {
  it('shows the §7.13 table with its type and status badges', async () => {
    api = mockApi(ROUTES);
    render('company_admin');
    expect(await screen.findByText('Alarm tetiklendi')).toBeVisible();
    expect(screen.getByText('Alarmlar')).toBeVisible();
    expect(screen.getByText('Uyarı')).toBeVisible();
  });

  it('sends the search term and both filters to the API (R226)', async () => {
    api = mockApi(ROUTES);
    render('company_admin');
    await screen.findByText('Alarm tetiklendi');

    await userEvent.type(screen.getByRole('searchbox', { name: 'Arama' }), 'tetiklendi');
    await userEvent.click(screen.getByRole('combobox', { name: /Tip/ }));
    await userEvent.click(await screen.findByRole('option', { name: 'Alarmlar' }));
    await userEvent.click(screen.getByRole('combobox', { name: /Durum/ }));
    await userEvent.click(await screen.findByRole('option', { name: 'Uyarı' }));

    await waitFor(() => {
      const calls = api.calls.filter((c) => new URL(c.url).pathname === '/api/v1/messages');
      const q = queryOf(api.calls, 'GET', '/api/v1/messages', calls.length - 1);
      expect(q.get('q')).toBe('tetiklendi');
      expect(q.get('kind')).toBe('alarm');
      expect(q.get('status')).toBe('warning');
    }, { timeout: 5000 });
  });

  it('hides the job-history tab from a role without jobs.runs.read, and never asks for it', async () => {
    api = mockApi(ROUTES);
    render('building_admin');
    await screen.findByText('Alarm tetiklendi');

    expect(screen.queryByRole('tab', { name: 'İş Geçmişi' })).toBeNull();
    expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/job-runs')).toBe(false);
  });

  it('shows the job history to a company admin without any trigger', async () => {
    api = mockApi(ROUTES);
    render('company_admin');
    await userEvent.click(await screen.findByRole('tab', { name: 'İş Geçmişi' }));

    expect(await screen.findByText('alarm.evaluate')).toBeVisible();
    expect(screen.getByText('Kısmi')).toBeVisible();
    expect(screen.queryByRole('button', { name: /Şimdi çalıştır/ })).toBeNull();
  });

  it('lets an admin trigger a job and watches it (R220, R192)', async () => {
    api = mockApi(ROUTES);
    render('admin');
    await userEvent.click(await screen.findByRole('tab', { name: 'İş Geçmişi' }));

    await userEvent.click(await screen.findByRole('button', { name: /alarm\.evaluate — Şimdi çalıştır/ }));
    await waitFor(() =>
      expect(api.calls.some((c) =>
        c.method === 'POST' && new URL(c.url).pathname === '/api/v1/job-runs/alarm.evaluate/trigger')).toBe(true));
    // The job is then watched through GET /jobs/{id}, the F6b convention.
    await waitFor(() =>
      expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/jobs/task-1')).toBe(true));
  });

  it('offers only the allow-listed jobs', async () => {
    api = mockApi(ROUTES);
    render('admin');
    await userEvent.click(await screen.findByRole('tab', { name: 'İş Geçmişi' }));

    const buttons = await screen.findAllByRole('button', { name: /Şimdi çalıştır/ });
    expect(buttons).toHaveLength(4);
    // R220: nothing outside the list, and never a task type the server refuses.
    expect(screen.queryByRole('button', { name: /system\.noop/ })).toBeNull();
    expect(screen.queryByRole('button', { name: /alarm\.notify/ })).toBeNull();
  });

  it('shows the empty state when nothing matches the filters', async () => {
    api = mockApi({ ...ROUTES, 'GET /api/v1/messages': { items: [], next_cursor: null } });
    render('company_admin');
    expect(await screen.findByText('Henüz mesaj yok.')).toBeVisible();
  });
});
