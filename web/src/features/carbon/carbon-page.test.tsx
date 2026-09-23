import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

const searchParams = vi.hoisted(() => ({ current: new URLSearchParams() }));
vi.mock('next/navigation', async () => {
  const { mockPathname: pathname, mockRouter: router } = await import('@/test/navigation');
  return {
    usePathname: () => pathname.current,
    useRouter: () => router,
    useSearchParams: () => searchParams.current,
    redirect: vi.fn(),
  };
});

const { CarbonPage } = await import('./carbon-page');

const me = (role: keyof typeof fixture): MeResponse => ({
  id: `u-${role}`,
  name: 'Ayşe Kaya',
  email: 'ayse@ornek.com.tr',
  role,
  locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'],
  session_id: 's-1',
  company: { id: 'c-1', name: 'Anadolu Tekstil' },
});

const CATALOGUE = {
  items: [
    { key: 'cat_waste', subs: [{ key: 'sub_waste_disposal', scope: 'scope_3', iso_category: 'category_6' }] },
  ],
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => {
  api = mockApi({
    'GET /api/v1/buildings': { items: [], next_cursor: null },
    'GET /api/v1/analyzers': { items: [], next_cursor: null },
    'GET /api/v1/carbon/activity-catalogue': CATALOGUE,
  });
});
afterEach(() => {
  api.restore();
  searchParams.current = new URLSearchParams();
});

const render = (role: keyof typeof fixture = 'company_admin') =>
  renderWithProviders(
    <SessionProvider me={me(role)}>
      <CarbonPage />
    </SessionProvider>,
  );

describe('CarbonPage', () => {
  it('opens the tab the URL names', async () => {
    searchParams.current = new URLSearchParams('tab=ghg');
    const r = render();
    expect(await r.findByRole('tab', { name: 'GHG Protokolü', selected: true })).toBeVisible();
  });

  it('falls back to the overview for an unknown tab', async () => {
    searchParams.current = new URLSearchParams('tab=nope');
    const r = render();
    expect(await r.findByRole('tab', { name: 'Genel Bakış', selected: true })).toBeVisible();
  });

  it('asks for a building on building sections (R321)', async () => {
    const r = render();
    expect(await r.findByText('Önce bir bina seçin')).toBeVisible();
  });

  it('says so when the role cannot read carbon data', () => {
    const r = renderWithProviders(
      <SessionProvider me={{ ...me('company_admin'), permissions: ['nav.core'] as MeResponse['permissions'] }}>
        <CarbonPage />
      </SessionProvider>,
    );
    expect(r.getByText('Bu modüle erişiminiz yok')).toBeVisible();
    expect(r.queryByRole('tab')).toBeNull();
  });
});
