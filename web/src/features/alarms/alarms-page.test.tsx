import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { demoAlarms, demoAnalyzers, demoEvaluation, demoEvents } from './_fixture';
import { AlarmsPage } from './alarms-page';

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
  'GET /api/v1/alarms': { items: demoAlarms, next_cursor: null },
  'GET /api/v1/analyzers': { items: demoAnalyzers, next_cursor: null },
  'GET /api/v1/alarms/al-1/events': { items: demoEvents, next_cursor: null },
  'GET /api/v1/alarms/al-2/events': { items: [], next_cursor: null },
  'POST /api/v1/alarms': Response.json({ ...demoAlarms[0], id: 'al-9' }, { status: 201 }),
  'PATCH /api/v1/alarms/al-1': Response.json(demoAlarms[0]),
  'PATCH /api/v1/alarms/al-2': Response.json(demoAlarms[1]),
  'DELETE /api/v1/alarms/al-1': new Response(null, { status: 204 }),
  'POST /api/v1/alarms/al-1/evaluate': Response.json(demoEvaluation),
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('AlarmsPage', () => {
  it('lists rules with their type and analyzers (§7.12)', async () => {
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <AlarmsPage />
      </SessionProvider>,
    );

    expect(await screen.findByText('Endüktif izleme')).toBeVisible();
    expect(screen.getByText('Reaktif Limit Algılama Alarmı')).toBeVisible();
    // R212: the power alarm's label no longer promises voltage or current.
    expect(screen.getByText('Veri İletişim Alarmı')).toBeVisible();
    expect(screen.getByText('A-1')).toBeVisible();
  });

  it('filters by active and passive', async () => {
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <AlarmsPage />
      </SessionProvider>,
    );
    await screen.findByText('Endüktif izleme');

    await userEvent.click(screen.getByRole('radio', { name: 'Pasif' }));
    await waitFor(() => {
      const calls = api.calls.filter((c) => new URL(c.url).pathname === '/api/v1/alarms');
      expect(queryOf(api.calls, 'GET', '/api/v1/alarms', calls.length - 1).get('is_enabled')).toBe('false');
    });
  });

  it('hides every write control from a read-only role', async () => {
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('company_readonly_admin')}>
        <AlarmsPage />
      </SessionProvider>,
    );
    await screen.findByText('Endüktif izleme');

    expect(screen.queryByRole('button', { name: 'Yeni Alarm Ekle' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Düzenle' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Sil' })).toBeNull();
    // alarms.evaluate stops at A and CA.
    expect(screen.queryByRole('button', { name: 'Şimdi değerlendir' })).toBeNull();
    // The details dialog is a read, so it stays.
    expect(screen.getAllByRole('button', { name: 'Detaylar' }).length).toBeGreaterThan(0);
  });

  it('lets a building admin edit but not evaluate', async () => {
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('building_admin')}>
        <AlarmsPage />
      </SessionProvider>,
    );
    await screen.findByText('Endüktif izleme');

    expect(screen.getByRole('button', { name: 'Yeni Alarm Ekle' })).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Şimdi değerlendir' })).toBeNull();
  });

  it('shows the dry-run verdicts and says nothing was sent (R223)', async () => {
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <AlarmsPage />
      </SessionProvider>,
    );
    await screen.findByText('Endüktif izleme');

    await userEvent.click(screen.getAllByRole('button', { name: 'Şimdi değerlendir' })[0]);
    expect(await screen.findByText(/Bu bir denemedir/)).toBeVisible();
    expect(screen.getByText('Endüktif oran %25 eşiği aştı (%20)')).toBeVisible();
  });

  it('renders a no-verdict as undecided, never as compliant (R216)', async () => {
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <AlarmsPage />
      </SessionProvider>,
    );
    await screen.findByText('Endüktif izleme');

    await userEvent.click(screen.getAllByRole('button', { name: 'Şimdi değerlendir' })[0]);
    expect(await screen.findByText(/Karar verilemedi/)).toBeVisible();
    expect(screen.getByText(/veri eksiktir/)).toBeVisible();
  });

  it('says SMS is not active yet when the channel is ticked (R211)', async () => {
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <AlarmsPage />
      </SessionProvider>,
    );

    await userEvent.click(await screen.findByRole('button', { name: 'Yeni Alarm Ekle' }));
    expect(screen.queryByText(/SMS bildirimi henüz etkin değil/)).toBeNull();

    await userEvent.click(screen.getByRole('checkbox', { name: 'SMS' }));
    expect(await screen.findByText(/SMS bildirimi henüz etkin değil/)).toBeVisible();
    expect(screen.getByLabelText(/SMS Numaraları/)).toBeVisible();
  });

  it('never offers a voltage field on the power alarm (R212)', async () => {
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <AlarmsPage />
      </SessionProvider>,
    );

    await userEvent.click(await screen.findByRole('button', { name: 'Yeni Alarm Ekle' }));
    // Radix renders a combobox trigger, not a <select>.
    await userEvent.click(screen.getByRole('combobox', { name: /Tip/ }));
    await userEvent.click(await screen.findByRole('option', { name: 'Güç Alarmı' }));

    expect(await screen.findByLabelText(/Güç Maks/)).toBeVisible();
    // R212: there is no voltage field to find, under any spelling.
    expect(screen.queryByLabelText(/Gerilim/)).toBeNull();
    expect(screen.queryByLabelText(/Voltaj/)).toBeNull();
  });

  it('refuses to save a rule with no limit (R230)', async () => {
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <AlarmsPage />
      </SessionProvider>,
    );

    await userEvent.click(await screen.findByRole('button', { name: 'Yeni Alarm Ekle' }));
    expect(screen.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
    expect(api.calls.some((c) => c.method === 'POST')).toBe(false);
  });

  it('shows the alarm log with its delivery state', async () => {
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <AlarmsPage />
      </SessionProvider>,
    );
    await screen.findByText('Endüktif izleme');

    await userEvent.click(screen.getAllByRole('button', { name: 'Detaylar' })[0]);
    expect(await screen.findByText('Gönderildi')).toBeVisible();
    // "fired and nobody was told" is its own state, not a missing row.
    expect(screen.getByText('Gönderilemedi')).toBeVisible();
  });

  it('surfaces a failed list request instead of an empty screen', async () => {
    api = mockApi({
      ...ROUTES,
      'GET /api/v1/alarms': Response.json(
        { error: { code: 'internal', message: 'Bir hata oluştu' } }, { status: 500 }),
    });
    renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <AlarmsPage />
      </SessionProvider>,
    );
    // The toaster speaks after the query's own retries give up, so this waits
    // longer than the default: without it a failed read looks like an empty
    // screen, which is the failure this test exists for.
    expect(await screen.findByText('Bir hata oluştu', {}, { timeout: 10_000 })).toBeVisible();
  });
});
