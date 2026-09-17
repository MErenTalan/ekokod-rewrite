import { waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import { selectionKey } from '@/lib/selection/selection-store';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';

import type { Bill } from '@/lib/api/types';
import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { LatestBillCard, LatestBillCardView } from './latest-bill-card';

const bill = {
  id: 'bill-1',
  scope: 'building',
  status: 'issued',
  period_key: '2026-08',
  active_import: '18450.500',
  total_cost: '48250.75',
} as Bill;

describe('LatestBillCardView', () => {
  it('shows the period, consumption and amount in Turkish figures', () => {
    const r = renderWithProviders(<LatestBillCardView bill={bill} scopeLabel="A1 Fabrika" />);
    expect(r.getByText('Ağustos 2026')).toBeInTheDocument();
    expect(r.getByText('18.450,5 kWh')).toBeInTheDocument();
    expect(r.getByText('₺48.250,75')).toBeInTheDocument();
    expect(r.getByText('Kesinleşti')).toBeInTheDocument();
    expect(r.getByRole('link', { name: 'Faturaya git' })).toHaveAttribute('href', '/ekorm/bills?bill=bill-1');
  });

  it('says so when no bill has been computed yet', () => {
    const r = renderWithProviders(<LatestBillCardView bill={null} scopeLabel="Şirket geneli" />);
    expect(r.getByText('Henüz hesaplanmış fatura yok')).toBeInTheDocument();
    expect(r.queryByRole('link')).toBeNull();
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(<LatestBillCardView bill={bill} scopeLabel="A1 Fabrika" />);
    await expectNoAxeViolations(r.container);
  });
});

describe('LatestBillCard', () => {
  it('asks for the selected building, and for the company when none is selected', async () => {
    localStorage.clear();
    const api = mockApi({ 'GET /api/v1/bills/latest': bill });
    const me: MeResponse = {
      id: 'u-ca',
      name: 'Ayşe Kaya',
      email: 'ayse@ornek.com.tr',
      role: 'company_admin',
      locale: 'tr',
      permissions: fixture.company_admin as MeResponse['permissions'],
      session_id: 's',
      company: { id: 'c-own', name: 'Kendi Şirketim' },
    };
    const company = renderWithProviders(
      <SessionProvider me={me}>
        <LatestBillCard />
      </SessionProvider>,
    );
    await waitFor(() => expect(api.calls.length).toBeGreaterThan(0));
    expect(queryOf(api.calls, 'GET', '/api/v1/bills/latest').get('scope')).toBe('company');
    expect(queryOf(api.calls, 'GET', '/api/v1/bills/latest').get('subject_id')).toBe('c-own');
    company.unmount();

    localStorage.setItem(selectionKey('u-ca'), JSON.stringify({ buildingId: 'b-1' }));
    const building = renderWithProviders(
      <SessionProvider me={me}>
        <LatestBillCard buildingName="A1 Fabrika" />
      </SessionProvider>,
    );
    await waitFor(() => expect(building.getByText('A1 Fabrika')).toBeInTheDocument());
    const last = queryOf(api.calls, 'GET', '/api/v1/bills/latest', 1);
    expect(last.get('scope')).toBe('building');
    expect(last.get('subject_id')).toBe('b-1');
    api.restore();
  });
});
