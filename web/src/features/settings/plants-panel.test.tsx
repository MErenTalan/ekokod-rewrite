import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { PlantsPanel } from './plants-panel';

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

const plant = { id: 'p-1', name: 'Çatı GES', plant_kind: 'rooftop', total_capacity_kw: '250.00', created_at: '' };

const ROUTES = {
  'GET /api/v1/power-plants': { items: [plant] },
  'GET /api/v1/power-plants/p-1': {
    ...plant,
    monthly_targets: Array.from({ length: 12 }, () => '1000'),
    alarm_recipients: [],
    devices: [],
  },
  'POST /api/v1/power-plants': Response.json({ id: 'p-2' }, { status: 201 }),
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => localStorage.clear());
afterEach(() => api.restore());

describe('PlantsPanel', () => {
  it('cannot save a new plant until every month has a target', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <PlantsPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByRole('button', { name: 'Santral ekle' })).toBeInTheDocument());
    await r.user.click(r.getByRole('button', { name: 'Santral ekle' }));
    expect(await r.findByText('12 ayın hedefi girilmeli')).toBeInTheDocument();
    expect(r.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
    expect(api.calls.some((c) => c.method === 'POST')).toBe(false);
  });

  it('shows a read-only role the plants without any control', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_readonly_admin')}>
        <PlantsPanel />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByRole('row', { name: /Çatı GES/ })).toBeInTheDocument());
    expect(r.queryByRole('button', { name: 'Santral ekle' })).toBeNull();
  });
});
