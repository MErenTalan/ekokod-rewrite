import { describe, expect, it } from 'vitest';

import fixture from '@/lib/session/permissions.fixture.json';

import { findTrail, isActive, isGroup, navFor, navigation, type NavEntry } from './nav-config';

const leafIds = (entries: NavEntry[]) => entries.flatMap((e) => (isGroup(e) ? e.children : [e])).map((l) => l.id);
const ALL = leafIds(navigation);
const WITHOUT_RESTRICTED = ALL.filter((id) => id !== 'solarPlants' && id !== 'financial');

describe('nav config', () => {
  it('finds the group and leaf for a route and its sub-routes', () => {
    expect(findTrail('/ekorm/alarms/ai')?.leaf.id).toBe('alarmsAi');
    expect(findTrail('/ekorm/alarms/ai')?.group?.id).toBe('alarms');
    expect(findTrail('/ekorm/bills/2026-08')?.leaf.id).toBe('bills');
    expect(findTrail('/ekorm/reports')?.group).toBeUndefined();
    expect(findTrail('/ekorm')?.leaf.id).toBe('dashboard');
    expect(findTrail('/ekorm/unknown')).toBeNull();
  });

  it('does not treat a shared prefix as active', () => {
    expect(isActive('/ekorm/billsx', '/ekorm/bills')).toBe(false);
  });

  it('every href lives under /ekorm and Calendar follows Reports (R167)', () => {
    const leaves = navigation.flatMap((e) => (isGroup(e) ? e.children : [e]));
    expect(leaves.every((l) => l.href === '/ekorm' || l.href.startsWith('/ekorm/'))).toBe(true);
    expect(navigation.map((e) => e.id).slice(4, 7)).toEqual(['reports', 'calendar', 'settings']);
  });

  it('disabled entries are exactly those 07 §7 marks', () => {
    const disabled = navigation.flatMap((e) => ('children' in e ? e.children : [e])).filter((l) => l.disabled).map((l) => l.id);
    expect(disabled).toEqual(['water', 'gas', 'evDrivers', 'alarmsAi', 'savingActions']);
  });
});

// The fixture is auth.PermissionsFor per role (Go TestPermissionsFixtureMatchesTable keeps it exact).
describe('navFor', () => {
  it.each([
    ['admin', ALL],
    ['company_admin', ALL],
    ['company_readonly_admin', ALL],
    ['building_admin', WITHOUT_RESTRICTED],
    ['building_readonly_admin', WITHOUT_RESTRICTED],
    ['demo', WITHOUT_RESTRICTED],
  ] as const)('%s sees exactly its navigation', (role, want) => {
    expect(leafIds(navFor(fixture[role]))).toEqual(want);
  });

  it('keeps disabled placeholders visible for everyone (01 §5)', () => {
    expect(leafIds(navFor(fixture.demo))).toContain('water');
  });

  // R228: all six roles hold both permissions today, so navFor's output is
  // unchanged. The point is that the leaves DECLARE them, so a later narrowing
  // of alarms.read or messages.read moves the navigation with it.
  it('gates the alarm and message leaves on their own permissions (R228)', () => {
    const leaves = navigation.flatMap((e) => (isGroup(e) ? e.children : [e]));
    expect(leaves.find((l) => l.href === '/ekorm/alarms')?.permission).toBe('alarms.read');
    expect(leaves.find((l) => l.href === '/ekorm/messages')?.permission).toBe('messages.read');
    expect(leafIds(navFor(['nav.core']))).not.toContain('alarmsManual');
    expect(leafIds(navFor(['nav.core']))).not.toContain('messages');
  });

  // R246: all six roles hold both permissions today, so navFor's output is
  // unchanged. Declaring them is what makes a later narrowing move the
  // navigation with it — and what keeps a principal who loses one off the
  // screen that would 403 anyway.
  it('gates the bills and tariffs leaves on their own permissions (R246)', () => {
    const leaves = navigation.flatMap((e) => (isGroup(e) ? e.children : [e]));
    expect(leaves.find((l) => l.href === '/ekorm/bills')?.permission).toBe('bills.read');
    expect(leaves.find((l) => l.href === '/ekorm/tariffs')?.permission).toBe('tariffs.read');
    expect(leafIds(navFor(['nav.core', 'tariffs.read']))).not.toContain('bills');
    expect(leafIds(navFor(['nav.core', 'bills.read']))).not.toContain('tariffs');
  });

  it('hides reports without reports.read (F8b)', () => {
    const leaves = navigation.flatMap((e) => (isGroup(e) ? e.children : [e]));
    expect(leaves.find((l) => l.href === '/ekorm/reports')?.permission).toBe('reports.read');
    expect(leafIds(navFor(['nav.core']))).not.toContain('reports');
    expect(leafIds(navFor(['nav.core', 'reports.read']))).toContain('reports');
  });

  it('drops a group left empty and shows nothing without nav.core', () => {
    expect(navFor([])).toEqual([]);
    const onlySolar = navFor(['nav.solar_plants']);
    expect(onlySolar.map((e) => e.id)).toEqual(['dataAnalysis']);
    expect(leafIds(onlySolar)).toEqual(['solarPlants']);
  });
});

describe('F9 routes (R295)', () => {
  it('uses the 01 §5 paths for the generation screens', () => {
    const leaves = navigation.flatMap((e) => ('children' in e ? e.children : [e]));
    const href = (id: string) => leaves.find((l) => l.id === id)?.href;
    expect(href('solarPlants')).toBe('/ekorm/solar-plants');
    expect(href('financial')).toBe('/ekorm/financial-analysis');
    expect(href('renewable')).toBe('/ekorm/renewable-energy');
  });
});
