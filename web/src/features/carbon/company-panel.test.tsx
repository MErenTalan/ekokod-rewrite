import { afterEach, describe, expect, it } from 'vitest';

import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { me } from './_fixture';
import { CompanyPanel } from './company-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

const company = { id: 'c-1', name: 'Anadolu Tekstil', address: 'Bursa', created_at: '2026-01-01T00:00:00+03:00', updated_at: '2026-01-01T00:00:00+03:00' };

describe('CompanyPanel', () => {
  it('reads the company record for a company admin', async () => {
    api = mockApi({ 'GET /api/v1/companies/c-1': company });
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <CompanyPanel />
      </SessionProvider>,
    );
    expect(await r.findByText('Bursa')).toBeVisible();
  });

  it('never asks for the record for a building admin', () => {
    api = mockApi({});
    const r = renderWithProviders(
      <SessionProvider me={me('building_admin')}>
        <CompanyPanel />
      </SessionProvider>,
    );
    expect(r.getByText('Anadolu Tekstil')).toBeVisible();
    expect(api.calls).toHaveLength(0);
  });
});
