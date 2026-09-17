import { describe, expect, it } from 'vitest';

import fixture from './permissions.fixture.json';

// R191: the fixture mirrors auth.PermissionsFor (Go guard TestPermissionsFixtureMatchesTable);
// these are the two permissions F6b's screens gate on beyond F6a's table.
describe('permission fixture', () => {
  it('gives analyzer editing to admin and company admin only', () => {
    expect(fixture.admin).toContain('settings.analyzers.edit');
    expect(fixture.company_admin).toContain('settings.analyzers.edit');
    for (const role of ['company_readonly_admin', 'building_admin', 'building_readonly_admin', 'demo'] as const) {
      expect(fixture[role]).not.toContain('settings.analyzers.edit');
    }
  });

  it('gives the anomaly check to the three writing roles', () => {
    for (const role of ['admin', 'company_admin', 'building_admin'] as const) {
      expect(fixture[role]).toContain('anomaly.check');
    }
    for (const role of ['company_readonly_admin', 'building_readonly_admin', 'demo'] as const) {
      expect(fixture[role]).not.toContain('anomaly.check');
    }
  });
});
