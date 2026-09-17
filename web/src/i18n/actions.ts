'use server';

import { cookies } from 'next/headers';

import { LOCALE_COOKIE, resolveLocale, type Locale } from './locale';

/** Cookie locale, no URL prefix (plan D8); the caller refreshes the router afterwards. */
export async function setLocale(locale: Locale): Promise<void> {
  (await cookies()).set(LOCALE_COOKIE, resolveLocale(locale), { path: '/', maxAge: 31_536_000, sameSite: 'lax' });
}
