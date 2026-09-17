'use client';

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';

import { htmlAttributes, serializeUiPreferences, UI_COOKIE, type UiPreferences } from './ui-preferences';

type Value = { prefs: UiPreferences; update: (patch: Partial<UiPreferences>) => void };

const UiPreferencesContext = createContext<Value | null>(null);

function applyToDocument(prefs: UiPreferences) {
  const { style, ...data } = htmlAttributes(prefs);
  const html = document.documentElement;
  for (const [name, value] of Object.entries(data)) html.setAttribute(name, value);
  html.style.setProperty('--radius-scale', String((style as Record<string, string>)['--radius-scale']));
}

/** Mounted once in the root layout with the cookie value (plan I-15); F6 syncs changes to the user profile. */
export function UiPreferencesProvider({ initial, children }: { initial: UiPreferences; children: ReactNode }) {
  const [prefs, setPrefs] = useState(initial);
  const update = useCallback(
    (patch: Partial<UiPreferences>) => {
      const next = { ...prefs, ...patch };
      document.cookie = `${UI_COOKIE}=${serializeUiPreferences(next)}; path=/; max-age=31536000; SameSite=Lax`;
      applyToDocument(next);
      setPrefs(next);
    },
    [prefs],
  );
  const value = useMemo(() => ({ prefs, update }), [prefs, update]);
  return <UiPreferencesContext.Provider value={value}>{children}</UiPreferencesContext.Provider>;
}

export function useUiPreferences(): Value {
  const value = useContext(UiPreferencesContext);
  if (!value) throw new Error('useUiPreferences must be used inside <UiPreferencesProvider>');
  return value;
}
