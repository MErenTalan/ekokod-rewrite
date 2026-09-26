import { describe, expect, it } from 'vitest';

import { leavesOf, navFor } from '@/components/shell/nav';
import { TABS } from '@/features/carbon/labels';
import fixture from '@/lib/session/permissions.fixture.json';

import { ekocmNav } from './nav';
import { SECTION_HREF } from './sections';

describe('EKOCM nav (R488)', () => {
  it('gives every former tab exactly one page under /ekocm', () => {
    expect(Object.keys(SECTION_HREF).sort()).toEqual([...TABS].sort());
    const hrefs = leavesOf(ekocmNav).map((l) => l.href);
    expect(hrefs.sort()).toEqual(Object.values(SECTION_HREF).sort());
    expect(hrefs.every((h) => h === '/ekocm' || h.startsWith('/ekocm/'))).toBe(true);
  });

  it('gates every leaf on carbon.read', () => {
    expect(leavesOf(ekocmNav).every((l) => l.permission === 'carbon.read')).toBe(true);
    expect(navFor(ekocmNav, ['nav.core'])).toEqual([]);
  });

  it.each(Object.keys(fixture) as (keyof typeof fixture)[])('%s sees the whole module', (role) => {
    expect(leavesOf(navFor(ekocmNav, fixture[role]))).toHaveLength(TABS.length);
  });
});
