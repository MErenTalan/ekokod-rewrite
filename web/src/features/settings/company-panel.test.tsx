import { waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { selectionKey } from '@/lib/selection/selection-store';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { CompanyPanel } from './company-panel';

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

const company = {
  id: 'c-own',
  name: 'Kendi Şirketim',
  analyzer_counts: [{ provider: 'osos', subtype: 'Baskent', count: 2 }],
  created_at: '',
  updated_at: '',
};

const ROUTES = {
  'GET /api/v1/companies/c-own': company,
  'GET /api/v1/companies/c-other': { ...company, id: 'c-other', name: 'Diğer Şirket' },
  'GET /api/v1/companies': { items: [] },
  'GET /api/v1/integration-definitions': { items: [] },
  'GET /api/v1/integration-credentials': { items: [] },
  'GET /api/v1/buildings': { items: [] },
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('CompanyPanel', () => {
  it('reads the company the admin is acting for (R139)', async () => {
    localStorage.setItem(selectionKey('u-admin'), JSON.stringify({ companyId: 'c-other' }));
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('admin')}>
        <CompanyPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByDisplayValue('Diğer Şirket')).toBeInTheDocument());
    expect(queryOf(api.calls, 'GET', '/api/v1/companies/c-other').get('company_id')).toBe('c-other');
  });

  it('shows the company list and the credential creation only to an admin', async () => {
    api = mockApi(ROUTES);
    const admin = renderWithProviders(
      <SessionProvider me={me('admin')}>
        <CompanyPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(admin.getByRole('button', { name: 'Şirket ekle' })).toBeInTheDocument());
    expect(admin.getByRole('button', { name: 'Entegrasyon ekle' })).toBeInTheDocument();
    admin.unmount();

    api.restore();
    api = mockApi(ROUTES);
    const companyAdmin = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <CompanyPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(companyAdmin.getByText('Entegrasyonlar')).toBeInTheDocument());
    expect(companyAdmin.queryByRole('button', { name: 'Şirket ekle' })).toBeNull();
    expect(companyAdmin.queryByRole('button', { name: 'Entegrasyon ekle' })).toBeNull();
  });

  it('shows the verification it just made, not the list it read before', async () => {
    const user = userEvent.setup();
    let verifiedAt: string | null = null;
    const credential = () => ({
      id: 'cr-1', definition_id: 'd-1', provider: 'pm5340', subtype: 'PM5340', username: null, has_secret: false,
      extra_keys: [], is_active: true, pm5340_url: 'http://10.0.0.5:8080', last_verified_at: verifiedAt, updated_at: '',
    });
    api = mockApi({
      ...ROUTES,
      'GET /api/v1/integration-credentials': () => Response.json({ items: [credential()] }),
      'POST /api/v1/integration-credentials/cr-1/verify': () => {
        verifiedAt = '2026-10-02T15:00:00+03:00';
        return Response.json(credential());
      },
    });
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <CompanyPanel />
      </SessionProvider>,
    );
    const row = await r.findByRole('row', { name: /pm5340/ });
    expect(row).toHaveTextContent('Hiç');
    // A gateway has no password: its address is what identifies it.
    expect(row).toHaveTextContent('http://10.0.0.5:8080');
    expect(row).toHaveTextContent('Parola gerekmez');

    await user.click(r.getByRole('button', { name: /İşlemler|Satır işlemleri|Eylemler/ }));
    await user.click(await r.findByRole('menuitem', { name: 'Doğrula' }));
    await waitFor(() => expect(r.getByRole('row', { name: /pm5340/ })).toHaveTextContent('2 Eki 2026'));
  });

  it('hides the integrations from a read-only role', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_readonly_admin')}>
        <CompanyPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByText('Şirket bilgileri')).toBeInTheDocument());
    expect(r.queryByText('Entegrasyonlar')).toBeNull();
    expect(api.calls.some((c) => c.url.includes('/integration-credentials'))).toBe(false);
  });
});
