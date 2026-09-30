import { describe, expect, it } from 'vitest';

import { SITE_PAGES } from '@/lib/site/paths';

import { LEGACY_ROOTS, moduleRedirect } from './redirects';

describe('moduleRedirect (F17 R493)', () => {
  it.each([
    // The tabbed carbon page's deep links land on their own EKOCM page.
    ['/ekorm/carbon-footprint', '', '/ekocm'],
    ['/ekorm/carbon-footprint', '?tab=status', '/ekocm/activity-status'],
    ['/ekorm/carbon-footprint', '?tab=database&year=2025', '/ekocm/emission-factors?year=2025'],
    ['/ekorm/carbon-footprint', '?tab=nope', '/ekocm'],
    ['/ekorm/carbon-footprint', '?tab=constructor', '/ekocm'],
    ['/ekorm/carbon-footprint/x', '', '/ekocm'],
    // Legacy root paths move under their module.
    ['/carbon-footprint', '', '/ekocm'],
    ['/consumption', '?x=1', '/ekorm/consumption?x=1'],
    ['/bills/2026-08', '', '/ekorm/bills/2026-08'],
    ['/bills-and-tariffs', '', '/ekorm/bills'],
    ['/settings/account', '', '/ekorm/settings'],
    ['/iso-50001', '', '/ekorm/iso-50001'],
    ['/ai', '', '/ekorm/ai'],
    // The legacy EKOCM app's own login is the shared one now.
    ['/ekocm/auth/login', '', '/auth/login?next=%2Fekocm'],
  ])('%s%s → %s', (path, search, want) => {
    expect(moduleRedirect(path, search)).toBe(want);
  });

  it.each(['/', '/contact', '/ekorm', '/ekorm/consumption', '/ekocm', '/ekocm/data-entry', '/consumptionx', '/auth/login', '/modules'])(
    'leaves %s alone',
    (path) => {
      expect(moduleRedirect(path, '')).toBeNull();
    },
  );

  it('never shadows a public site page', () => {
    expect(Object.keys(LEGACY_ROOTS).filter((seg) => (SITE_PAGES as readonly string[]).includes(`/${seg}`))).toEqual([]);
  });
});
