import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { UsersPanel } from './users-panel';

const me = (role: keyof typeof fixture): MeResponse => ({
  id: 'u-1',
  name: 'Ayşe Kaya',
  email: 'ayse@ornek.com.tr',
  role,
  locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'],
  session_id: 's',
  company: { id: 'c-own', name: 'Kendi Şirketim' },
});

const ROUTES = {
  'GET /api/v1/users': {
    items: [
      { id: 'u-1', name: 'Ayşe Kaya', email: 'ayse@ornek.com.tr', role: 'company_admin', is_active: true, locale: 'tr', created_at: '' },
    ],
  },
  'POST /api/v1/users': Response.json({ id: 'u-2' }, { status: 201 }),
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('UsersPanel', () => {
  it('creates a user with the role a company admin may assign', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <UsersPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByRole('button', { name: 'Kullanıcı ekle' })).toBeInTheDocument());
    await r.user.click(r.getByRole('button', { name: 'Kullanıcı ekle' }));
    await r.user.type(await r.findByLabelText(/Ad soyad/), 'Yeni Kullanıcı');
    await r.user.type(r.getByLabelText(/E-posta/), 'yeni@ornek.com.tr');
    await r.user.type(r.getByLabelText(/Şifre/), 'Guvenli!Sifre-42');
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'POST')).toBe(true));
    const body = await api.calls.find((c) => c.method === 'POST')!.clone().json();
    expect(body).toMatchObject({ name: 'Yeni Kullanıcı', email: 'yeni@ornek.com.tr', role: 'building_readonly_admin' });
  });

  it('gives a read-only role the list alone', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_readonly_admin')}>
        <UsersPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByRole('row', { name: /Ayşe Kaya/ })).toBeInTheDocument());
    expect(r.queryByRole('button', { name: 'Kullanıcı ekle' })).toBeNull();
  });
});
