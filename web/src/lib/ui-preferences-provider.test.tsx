import { act, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it } from 'vitest';

import { defaultUiPreferences } from './ui-preferences';
import { UiPreferencesProvider, useUiPreferences } from './ui-preferences-provider';

const wrapper = ({ children }: { children: ReactNode }) => <UiPreferencesProvider initial={defaultUiPreferences}>{children}</UiPreferencesProvider>;

describe('UiPreferencesProvider', () => {
  afterEach(() => {
    document.cookie = 'ekokod_ui=; max-age=0; path=/';
    document.documentElement.removeAttribute('data-layout');
  });

  it('update writes the cookie and mirrors attributes onto <html>', () => {
    const { result } = renderHook(() => useUiPreferences(), { wrapper });
    act(() => result.current.update({ layout: 'horizontal', radiusScale: 1.5 }));
    expect(result.current.prefs.layout).toBe('horizontal');
    expect(document.documentElement.dataset.layout).toBe('horizontal');
    expect(document.documentElement.style.getPropertyValue('--radius-scale')).toBe('1.5');
    expect(document.cookie).toContain('ekokod_ui=');
    expect(decodeURIComponent(document.cookie)).toContain('"layout":"horizontal"');
  });

  it('throws outside the provider', () => {
    expect(() => renderHook(() => useUiPreferences())).toThrow(/UiPreferencesProvider/);
  });
});
