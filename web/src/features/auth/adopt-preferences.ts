import { setLocale } from '@/i18n/actions';
import { resolveLocale } from '@/i18n/locale';
import type { MeResponse } from '@/lib/api/errors';
import { parseUiPreferences, serializeUiPreferences, UI_COOKIE } from '@/lib/ui-preferences';

/** After sign-in the profile wins: its customiser state and locale become the SSR cookies (R169). */
export async function adoptProfilePreferences(me: Pick<MeResponse, 'ui_preferences' | 'locale'>): Promise<void> {
  if (me.ui_preferences) {
    const prefs = serializeUiPreferences(parseUiPreferences(me.ui_preferences));
    document.cookie = `${UI_COOKIE}=${prefs}; path=/; max-age=31536000; SameSite=Lax`;
  }
  await setLocale(resolveLocale(me.locale));
}
