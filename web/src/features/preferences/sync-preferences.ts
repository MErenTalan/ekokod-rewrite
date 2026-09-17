'use client';

import { useLocale } from 'next-intl';
import { useEffect, useMemo } from 'react';

import { api } from '@/lib/api/client';
import type { MeResponse } from '@/lib/api/errors';
import { parseUiPreferences, serializeUiPreferences } from '@/lib/ui-preferences';
import { useUiPreferences } from '@/lib/ui-preferences-provider';

export type PreferenceBody = { ui_preferences: string; locale: 'tr' | 'en' };

/** Coalesces bursts of preference changes (a radius slider) into one PATCH after `delayMs` of quiet. */
export function createPreferenceSync(send: (body: PreferenceBody) => Promise<unknown>, initial: PreferenceBody, delayMs = 500) {
  let saved = JSON.stringify(initial);
  let timer: ReturnType<typeof setTimeout> | undefined;
  return {
    push(body: PreferenceBody) {
      clearTimeout(timer);
      const key = JSON.stringify(body);
      if (key === saved) return;
      timer = setTimeout(() => {
        saved = key;
        send(body).catch(() => {
          // The cookie still holds the change; the next change retries.
          saved = '';
        });
      }, delayMs);
    },
    cancel() {
      clearTimeout(timer);
    },
  };
}

/** R169: the customiser and locale follow the user across devices; the cookie stays the SSR source. */
export function usePreferenceSync(me: MeResponse): void {
  const { prefs } = useUiPreferences();
  const locale = useLocale() as PreferenceBody['locale'];
  const readOnlyProfile = me.role === 'demo';
  const sync = useMemo(
    () =>
      createPreferenceSync(
        async (body) => {
          const { response } = await api.PATCH('/api/v1/profile', { body });
          if (!response.ok) throw new Error(String(response.status));
        },
        { ui_preferences: serializeUiPreferences(parseUiPreferences(me.ui_preferences ?? undefined)), locale: me.locale ?? 'tr' },
      ),
    [me.ui_preferences, me.locale],
  );
  useEffect(() => {
    if (!readOnlyProfile) sync.push({ ui_preferences: serializeUiPreferences(prefs), locale });
  }, [sync, prefs, locale, readOnlyProfile]);
  useEffect(() => () => sync.cancel(), [sync]);
}
