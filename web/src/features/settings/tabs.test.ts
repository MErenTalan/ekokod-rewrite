import { describe, expect, it } from 'vitest';

import fixture from '@/lib/session/permissions.fixture.json';
import type { Permission } from '@/lib/session/permissions';

import { resolveTab, visibleTabs } from './tabs';

const canFor = (role: keyof typeof fixture) => (permission: Permission) =>
  (fixture[role] as string[]).includes(permission);

// 09 §F6 acceptance: "a user of each of the six roles ... sees exactly the
// navigation and tabs the matrix specifies" (01 §2 settings-tab matrix).
describe('settings tabs', () => {
  it('gives every role exactly the tabs 01 §2 lists', () => {
    expect(visibleTabs(canFor('admin'))).toEqual([
      'account',
      'integrations',
      'company',
      'buildings',
      'plants',
      'analyzers',
      'users',
      'smtp',
    ]);
    expect(visibleTabs(canFor('company_admin'))).toEqual(['account', 'company', 'buildings', 'plants', 'analyzers', 'users']);
    expect(visibleTabs(canFor('company_readonly_admin'))).toEqual([
      'account',
      'company',
      'buildings',
      'plants',
      'analyzers',
      'users',
    ]);
    expect(visibleTabs(canFor('building_admin'))).toEqual(['account', 'analyzers']);
    expect(visibleTabs(canFor('building_readonly_admin'))).toEqual(['account', 'analyzers']);
    expect(visibleTabs(canFor('demo'))).toEqual(['account']);
  });

  it('falls back to the account tab for anything the role may not open', () => {
    const companyAdmin = visibleTabs(canFor('company_admin'));
    expect(resolveTab('smtp', companyAdmin)).toBe('account');
    expect(resolveTab('integrations', companyAdmin)).toBe('account');
    expect(resolveTab('nonsense', companyAdmin)).toBe('account');
    expect(resolveTab(null, companyAdmin)).toBe('account');
    expect(resolveTab('buildings', companyAdmin)).toBe('buildings');
  });
});
