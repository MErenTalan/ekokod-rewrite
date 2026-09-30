import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { mockRouter } from '@/test/navigation';
import { renderWithProviders } from '@/test/render';

import type { CarbonTab } from './labels';

const { CarbonSection } = await import('./section-page');

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
afterEach(() => api.restore());

const render = (section: CarbonTab, role: keyof typeof fixture = 'company_admin') =>
  renderWithProviders(
    <SessionProvider me={me(role)}>
      <CarbonSection section={section} />
    </SessionProvider>,
  );

describe('CarbonSection (R488)', () => {
  it('renders one section as its own page, headed by its name', async () => {
    const r = render('ghg');
    expect(r.getByRole('heading', { level: 1, name: 'GHG Protokolü' })).toBeVisible();
    expect(r.queryByRole('tab')).toBeNull();
    expect(r.queryByText('Önce bir bina seçin')).toBeNull();
  });

  it('asks for a building on building sections (R321)', async () => {
    const r = render('entry');
    expect(r.getByRole('heading', { level: 1, name: 'Veri Girişi' })).toBeVisible();
    expect(await r.findByText('Önce bir bina seçin')).toBeVisible();
  });

  it('describes the module on the overview only', () => {
    expect(render('overview').getByText("GHG Protokolü ve ISO 14064'e göre kurumsal karbon muhasebesi.")).toBeVisible();
  });

  it('says so when the role cannot read carbon data', () => {
    const r = renderWithProviders(
      <SessionProvider me={{ ...me('company_admin'), permissions: ['nav.core'] as MeResponse['permissions'] }}>
        <CarbonSection section="status" />
      </SessionProvider>,
    );
    expect(r.getByText('Bu modüle erişiminiz yok')).toBeVisible();
    expect(mockRouter.push).not.toHaveBeenCalled();
  });
});
