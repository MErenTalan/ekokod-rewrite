import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { monthly, summary } from './_fixture';
import { FinancialPage } from './financial-page';

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

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

describe('FinancialPage', () => {
  it('loads the current year: summary and the monthly table', async () => {
    api = mockApi({
      'GET /api/v1/financial/summary': summary,
      'GET /api/v1/financial/monthly': monthly,
    });
    const r = renderWithProviders(
      <SessionProvider me={me('company_readonly_admin')}>
        <FinancialPage />
      </SessionProvider>,
    );
    expect(await r.findByText("12 ayın 9'u veri içeriyor")).toBeVisible();
    expect(r.getByRole('table', { name: 'Aylık finansal tablo' })).toBeVisible();
    const year = String(new Date().getFullYear());
    await waitFor(() =>
      expect(queryOf(api.calls, 'GET', '/api/v1/financial/monthly').get('year')).toBe(year),
    );
    expect(queryOf(api.calls, 'GET', '/api/v1/financial/summary').get('month')).toBeNull();
  });

  it('a building admin gets an explanation and no request', async () => {
    api = mockApi({});
    const r = renderWithProviders(
      <SessionProvider me={me('building_admin')}>
        <FinancialPage />
      </SessionProvider>,
    );
    expect(await r.findByText('Finansal analiz şirket düzeyindedir')).toBeVisible();
    expect(api.calls).toHaveLength(0);
  });
});
