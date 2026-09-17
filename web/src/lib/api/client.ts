import createClient, { type Client } from 'openapi-fetch';

import { LOCALE_COOKIE } from '@/i18n/locale';

import { authRedirectFor, errorCode, REFRESHABLE_CODES } from './errors';
import type { paths } from './schema';

export const REFRESH_PATH = '/api/v1/auth/refresh';
// Public auth calls report their own 401s to the form; they never trigger a refresh or a redirect.
const NO_SESSION_PATHS = new Set([
  REFRESH_PATH,
  '/api/v1/auth/login',
  '/api/v1/auth/forgot-password',
  '/api/v1/auth/reset-password',
]);

type FetchLike = (request: Request) => Promise<Response>;

export type ApiClientOptions = {
  baseUrl: string;
  fetch: FetchLike;
  navigate: (href: string) => void;
  currentPath: () => string;
  locale: () => string | undefined;
};

async function codeOf(response: Response): Promise<string | undefined> {
  if (response.status !== 401) return undefined;
  try {
    return errorCode(await response.clone().json());
  } catch {
    return undefined;
  }
}

/** A typed client that refreshes the session once, shared by concurrent 401s, and retries the request (R168). */
export function createApiClient(opts: ApiClientOptions): {
  client: Client<paths>;
  refreshOnce: () => Promise<boolean>;
} {
  let inflight: Promise<boolean> | null = null;
  const refreshOnce = () => {
    inflight ??= opts
      .fetch(
        new Request(new URL(REFRESH_PATH, opts.baseUrl), {
          method: 'POST',
          credentials: 'same-origin',
          headers: { 'Content-Type': 'application/json' },
          body: '{}',
        }),
      )
      .then((r) => r.ok)
      .catch(() => false)
      .finally(() => {
        inflight = null;
      });
    return inflight;
  };

  const authFetch: FetchLike = async (request) => {
    const locale = opts.locale();
    if (locale && !request.headers.has('Accept-Language'))
      request.headers.set('Accept-Language', locale);
    const retry = request.clone();
    const response = await opts.fetch(request);
    if (NO_SESSION_PATHS.has(new URL(request.url).pathname)) return response;
    let code = await codeOf(response);
    if (!code) return response;
    let final = response;
    if (REFRESHABLE_CODES.has(code) && (await refreshOnce())) {
      final = await opts.fetch(retry);
      code = await codeOf(final);
      if (!code) return final;
    }
    const href = authRedirectFor(code, opts.currentPath());
    if (href) opts.navigate(href);
    return final;
  };

  return {
    client: createClient<paths>({
      baseUrl: opts.baseUrl,
      fetch: authFetch,
      credentials: 'same-origin',
    }),
    refreshOnce,
  };
}

function cookieValue(name: string): string | undefined {
  if (typeof document === 'undefined') return undefined;
  const hit = document.cookie.split('; ').find((c) => c.startsWith(`${name}=`));
  return hit ? decodeURIComponent(hit.slice(name.length + 1)) : undefined;
}

const browser = typeof window !== 'undefined';
const instance = createApiClient({
  // Same origin: Next rewrites /api/v1 to the API (R168). The origin is only a URL base for Request.
  baseUrl: browser ? window.location.origin : 'http://localhost',
  fetch: (request) => globalThis.fetch(request),
  navigate: (href) => window.location.assign(href),
  currentPath: () => (browser ? window.location.pathname + window.location.search : '/ekorm'),
  locale: () => cookieValue(LOCALE_COOKIE),
});

/** The browser API client. */
export const api = instance.client;
export const refreshOnce = instance.refreshOnce;
