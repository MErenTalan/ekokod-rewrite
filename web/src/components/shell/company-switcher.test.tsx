import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import { selectionKey, useSelection } from '@/lib/selection/selection-store';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { CompanySwitcher } from './company-switcher';

const me = (role: keyof typeof fixture): MeResponse => ({
  id: `user-${role}`,
  name: 'Ayşe Yılmaz',
  email: 'ayse@ornek.com.tr',
  role,
  locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'],
  session_id: 's',
  company: { id: 'platform', name: 'EKOKOD Platform' },
});

function Probe() {
  const s = useSelection();
  return <output data-testid="selection">{JSON.stringify({ companyId: s.companyId, buildingId: s.buildingId })}</output>;
}

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  localStorage.clear();
  fetchMock = vi.fn(async () => Response.json({ items: [{ id: 'c-a', name: 'Anadolu Tekstil' }, { id: 'c-b', name: 'Boğaziçi Gıda' }] }));
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => vi.unstubAllGlobals());

describe('CompanySwitcher', () => {
  it.each(['company_admin', 'company_readonly_admin', 'building_admin', 'demo'] as const)('renders nothing for %s', (role) => {
    const r = renderWithProviders(
      <SessionProvider me={me(role)}>
        <CompanySwitcher />
      </SessionProvider>,
    );
    expect(r.queryByRole('combobox')).toBeNull();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('lets an admin pick a company, which clears the building', async () => {
    localStorage.setItem(selectionKey('user-admin'), JSON.stringify({ buildingId: 'b-1' }));
    const r = renderWithProviders(
      <SessionProvider me={me('admin')}>
        <CompanySwitcher />
        <Probe />
      </SessionProvider>,
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(new URL((fetchMock.mock.calls[0][0] as Request).url).search).toBe('?limit=500');
    await r.user.click(r.getByRole('combobox', { name: 'Şirket' }));
    await r.user.click(await r.findByRole('option', { name: 'Boğaziçi Gıda' }));
    expect(r.getByTestId('selection')).toHaveTextContent('{"companyId":"c-b"}');
    expect(JSON.parse(localStorage.getItem(selectionKey('user-admin'))!)).toEqual({ companyId: 'c-b' });

    await r.user.click(r.getByRole('combobox', { name: 'Şirket' }));
    await r.user.click(await r.findByRole('option', { name: 'Kendi şirketim (EKOKOD Platform)' }));
    expect(r.getByTestId('selection')).toHaveTextContent('{}');
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(
      <SessionProvider me={me('admin')}>
        <CompanySwitcher />
      </SessionProvider>,
    );
    await expectNoAxeViolations(r.container);
  });
});
