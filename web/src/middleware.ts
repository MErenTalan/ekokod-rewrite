import { NextResponse, type NextRequest } from 'next/server';

import { authRedirectFor, errorCode } from '@/lib/api/errors';
import { isPublicPath, legacyRedirect } from '@/lib/site/paths';

export const ACCESS_COOKIE = 'ekokod_at';
export const REFRESH_COOKIE = 'ekokod_rt';
const RETRY_COOKIE = 'ekokod_retry';
const FORWARDED_HEADERS = ['cookie', 'user-agent', 'accept-language', 'x-forwarded-for'] as const;

function internalApi(): string {
  return process.env.EKOKOD_INTERNAL_API_URL ?? 'http://localhost:8080';
}

function redirectTo(request: NextRequest, href: string, setCookies: string[] = []): NextResponse {
  const res = NextResponse.redirect(new URL(href, request.url));
  for (const c of setCookies) res.headers.append('set-cookie', c);
  return res;
}

/** The request's cookie header with the refreshed cookies applied, so this render already sees them. */
function withCookies(header: string, setCookies: string[]): string {
  const jar = new Map(
    header
      .split(/;\s*/)
      .filter(Boolean)
      .map((pair) => {
        const i = pair.indexOf('=');
        return [pair.slice(0, i), pair.slice(i + 1)] as const;
      }),
  );
  for (const sc of setCookies) {
    const [pair] = sc.split(';');
    const i = pair.indexOf('=');
    jar.set(pair.slice(0, i).trim(), pair.slice(i + 1));
  }
  return [...jar].map(([k, v]) => `${k}=${v}`).join('; ');
}

/** PATH_HEADER carries the guarded path to the server layout (R208). */
export const PATH_HEADER = 'x-ekokod-path';

function withPath(request: NextRequest, path: string): Headers {
  const headers = new Headers(request.headers);
  headers.set(PATH_HEADER, path);
  return headers;
}

/** Guards every page: a missing access cookie is refreshed server-side before the render (R168). */
export async function middleware(request: NextRequest): Promise<NextResponse> {
  const { pathname, search } = request.nextUrl;
  const moved = legacyRedirect(pathname);
  if (moved) return NextResponse.redirect(new URL(moved, request.url), 308);
  // F12b: the marketing site needs no session; `/` is its homepage now (R167 retired).
  if (isPublicPath(pathname)) return NextResponse.next();
  if (pathname.startsWith('/auth/') || pathname === '/auth') return NextResponse.next();
  const next = pathname + search;
  // R208: a server component cannot read its own URL, so the guarded path travels
  // as a header and the layout's redirect can carry `next` like this one does.
  if (request.cookies.get(ACCESS_COOKIE)?.value) return NextResponse.next({ request: { headers: withPath(request, next) } });

  const login = authRedirectFor('session_revoked', next)!;
  if (!request.cookies.get(REFRESH_COOKIE)?.value) return redirectTo(request, login);

  const headers = new Headers({ 'content-type': 'application/json' });
  for (const name of FORWARDED_HEADERS) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  let response: Response;
  try {
    response = await fetch(`${internalApi()}/api/v1/auth/refresh`, {
      method: 'POST',
      headers,
      body: '{}',
      cache: 'no-store',
      redirect: 'manual',
    });
  } catch {
    return redirectTo(request, login);
  }
  const setCookies = response.headers.getSetCookie();
  if (response.ok) {
    const forwarded = withPath(request, next);
    forwarded.set('cookie', withCookies(request.headers.get('cookie') ?? '', setCookies));
    const res = NextResponse.next({ request: { headers: forwarded } });
    for (const c of setCookies) res.headers.append('set-cookie', c);
    return res;
  }

  const code = errorCode(await response.json().catch(() => null)) ?? 'session_revoked';
  if (code === 'token_rotated' && !request.cookies.get(RETRY_COOKIE)) {
    // A parallel request rotated first and its response carries the new cookies: retry once with them.
    const res = redirectTo(request, next);
    res.cookies.set(RETRY_COOKIE, '1', { path: '/', maxAge: 10, httpOnly: true, sameSite: 'lax' });
    return res;
  }
  return redirectTo(request, authRedirectFor(code, next) ?? login, setCookies);
}

export const config = { matcher: ['/((?!_next/|api/|favicon.ico|fonts/|.*\\..*).*)'] };
