import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { demoAnalyzers, demoDashboard } from './_fixture';
import { BillsPage } from './bills-page';

const me = (role: keyof typeof fixture): MeResponse => ({
  id: 'u-1', name: 'Ayşe Kaya', email: 'ayse@ornek.com.tr', role, locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'], session_id: 's',
  company: { id: 'c-own', name: 'Kendi Şirketim' },
});

const ROUTES = {
  'GET /api/v1/bills/dashboard': demoDashboard,
  'GET /api/v1/analyzers': { items: demoAnalyzers, next_cursor: null },
  'GET /api/v1/buildings': { items: [{ id: 'b-1', company_id: 'c-own', name: 'A1 Fabrika', bill_cutoff_day: 1, created_at: '', updated_at: '' }], total: 1 },
  'POST /api/v1/bills/compute': Response.json({ job_ids: ['billing.generate:company:c-own:2026-08'] }, { status: 202 }),
  // openapi-fetch percent-encodes the colons in the deterministic task id.
  'GET /api/v1/jobs/billing.generate%3Acompany%3Ac-own%3A2026-08': {
    id: 'billing.generate:company:c-own:2026-08', type: 'billing.generate', status: 'failed', error_code: 'tariff_not_found',
  },
};

let api: ReturnType<typeof mockApi>;
const render = (role: keyof typeof fixture = 'company_admin') =>
  renderWithProviders(
    <SessionProvider me={me(role)}>
      <BillsPage />
    </SessionProvider>,
  );

beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('BillsPage', () => {
  it('asks for a month before it fetches anything', async () => {
    api = mockApi(ROUTES);
    render();
    expect(await screen.findByText(/Bir ay seçin/)).toBeVisible();
    expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/bills/dashboard')).toBe(false);
  });

  it('shows the month’s rows and their totals once a month is chosen', async () => {
    const user = userEvent.setup();
    api = mockApi(ROUTES);
    render();
    await pickAugust(user);

    expect(await screen.findByRole('cell', { name: 'Sayaç 1' })).toBeVisible();
    expect(screen.getByRole('row', { name: /Ara toplam/ })).toHaveTextContent('510');
    await waitFor(() => {
      expect(queryOf(api.calls, 'GET', '/api/v1/bills/dashboard').get('year')).toBe('2026');
    });
    expect(queryOf(api.calls, 'GET', '/api/v1/bills/dashboard').get('month')).toBe('8');
  });

  it('lists the plant section beside the netting (R290)', async () => {
    const user = userEvent.setup();
    api = mockApi(ROUTES);
    render();
    await pickAugust(user);
    // R290: the plant section lists the company's plants beside the netting.
    expect(await screen.findByRole('region', { name: 'GES santralleri' })).toBeVisible();
    expect(screen.getByText('Konya GES')).toBeVisible();
  });

  it('offers no generation card to a principal without bills.compute', async () => {
    const user = userEvent.setup();
    api = mockApi(ROUTES);
    render('company_readonly_admin');
    await pickAugust(user);
    expect(await screen.findByRole('cell', { name: 'Sayaç 1' })).toBeVisible();
    expect(screen.queryByRole('button', { name: /Şirket faturası/ })).toBeNull();
  });

  it('offers the finished invoice for download once the job succeeds', async () => {
    const user = userEvent.setup();
    api = mockApi({
      ...ROUTES,
      'GET /api/v1/jobs/billing.generate%3Acompany%3Ac-own%3A2026-08': {
        id: 'billing.generate:company:c-own:2026-08', type: 'billing.generate', status: 'succeeded',
      },
      'GET /api/v1/bills': { items: [{ ...demoDashboard.buildings[0].rows[0], id: 'bill-9' }], total: 1 },
    });
    render();
    await pickAugust(user);

    await user.click(await screen.findByRole('button', { name: /Şirket faturası/ }));
    // R238: compute -> watch -> download; the finished invoice is offered here.
    expect(await screen.findByRole('button', { name: /Faturayı indir/ })).toBeVisible();
  });

  it('names the data condition that stopped a generation', async () => {
    const user = userEvent.setup();
    api = mockApi(ROUTES);
    render();
    await pickAugust(user);

    await user.click(await screen.findByRole('button', { name: /Şirket faturası/ }));
    expect(await screen.findByText(/Bina için tarife bulunamadı/)).toBeVisible();
  });
});

async function pickAugust(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: /Ay seçimi/ }));
  await user.click(await screen.findByRole('button', { name: /Ağustos/ }));
}
