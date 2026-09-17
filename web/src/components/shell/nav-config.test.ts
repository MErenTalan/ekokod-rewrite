import { describe, expect, it } from 'vitest';

import { findTrail, isActive, navigation } from './nav-config';

describe('nav config', () => {
  it('finds the group and leaf for a route and its sub-routes', () => {
    expect(findTrail('/alarms/ai')?.leaf.id).toBe('alarmsAi');
    expect(findTrail('/alarms/ai')?.group?.id).toBe('alarms');
    expect(findTrail('/bills/2026-08')?.leaf.id).toBe('bills');
    expect(findTrail('/reports')?.group).toBeUndefined();
    expect(findTrail('/unknown')).toBeNull();
  });

  it('does not treat a shared prefix as active', () => {
    expect(isActive('/billsx', '/bills')).toBe(false);
  });

  it('disabled entries are exactly those 07 §7 marks', () => {
    const disabled = navigation.flatMap((e) => ('children' in e ? e.children : [e])).filter((l) => l.disabled).map((l) => l.id);
    expect(disabled).toEqual(['water', 'gas', 'evDrivers', 'alarmsAi', 'savingActions']);
  });
});
