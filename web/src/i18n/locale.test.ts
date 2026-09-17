import { describe, expect, it } from 'vitest';

import { resolveLocale } from './locale';

describe('resolveLocale', () => {
  it('unknown or missing cookie falls back to tr', () => {
    expect(resolveLocale(undefined)).toBe('tr');
    expect(resolveLocale('de')).toBe('tr');
    expect(resolveLocale('en')).toBe('en');
  });
});
