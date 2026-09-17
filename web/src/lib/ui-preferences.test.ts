import { describe, expect, it } from 'vitest';

import { defaultUiPreferences, htmlAttributes, parseUiPreferences, serializeUiPreferences, type UiPreferences } from './ui-preferences';

describe('ui preferences', () => {
  it('garbage and missing cookies give defaults', () => {
    expect(parseUiPreferences(undefined)).toEqual(defaultUiPreferences);
    expect(parseUiPreferences('%%%')).toEqual(defaultUiPreferences);
    expect(parseUiPreferences('{"theme":"neon"}')).toEqual(defaultUiPreferences);
  });

  it('radius is clamped and quantised', () => {
    const radius = (r: unknown) => parseUiPreferences(JSON.stringify({ radiusScale: r })).radiusScale;
    expect(radius(9)).toBe(1.5);
    expect(radius(0.61)).toBe(0.5);
    expect(radius(1.1)).toBe(1);
    expect(radius('1.25')).toBe(1);
  });

  it('round trip', () => {
    const p: UiPreferences = { theme: 'dark', layout: 'horizontal', container: 'boxed', sidebar: 'collapsed', card: 'shadow', radiusScale: 1.25 };
    expect(parseUiPreferences(serializeUiPreferences(p))).toEqual(p);
  });

  it('html attributes mirror every preference', () => {
    expect(htmlAttributes({ ...defaultUiPreferences, card: 'shadow' })).toEqual({
      'data-theme': 'system',
      'data-layout': 'vertical',
      'data-container': 'full',
      'data-sidebar': 'expanded',
      'data-card': 'shadow',
      style: { '--radius-scale': '1' },
    });
  });
});
