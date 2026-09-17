import { cookies, headers } from 'next/headers';
import createClient, { type Client } from 'openapi-fetch';
import { cache } from 'react';

import { LOCALE_COOKIE } from '@/i18n/locale';

import type { MeResponse } from './errors';
import type { paths } from './schema';

const FORWARDED_HEADERS = ['cookie', 'user-agent', 'x-forwarded-for'] as const;

export function internalApiUrl(): string {
  return process.env.EKOKOD_INTERNAL_API_URL ?? 'http://localhost:8080';
}

/** A client for server code: the internal API URL with the browser's session and device headers (R168). */
export async function serverApi(): Promise<Client<paths>> {
  const [incoming, jar] = await Promise.all([headers(), cookies()]);
  const forwarded: Record<string, string> = {};
  for (const name of FORWARDED_HEADERS) {
    const value = incoming.get(name);
    if (value) forwarded[name] = value;
  }
  // API messages follow the app locale, not the browser's language list.
  const language = jar.get(LOCALE_COOKIE)?.value ?? incoming.get('accept-language');
  if (language) forwarded['accept-language'] = language;
  return createClient<paths>({
    baseUrl: internalApiUrl(),
    headers: forwarded,
    fetch: (request) => fetch(request, { cache: 'no-store' }),
  });
}

/** The signed-in user for this request, or null (deduplicated per render). */
export const getMe = cache(async (): Promise<MeResponse | null> => {
  try {
    const { data } = await (await serverApi()).GET('/api/v1/auth/me');
    return data ?? null;
  } catch {
    return null;
  }
});
