// @vitest-environment node
import { NextRequest } from 'next/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { middleware } from './middleware';

const INTERNAL = 'http://api.internal:18080';
const UA = 'Mozilla/5.0 Test';

function request(path: string, cookie?: string) {
  const headers = new Headers({
    'user-agent': UA,
    'accept-language': 'en',
    'x-forwarded-for': '203.0.113.7',
  });
  if (cookie) headers.set('cookie', cookie);
  return new NextRequest(`https://app.test${path}`, { headers });
}

const refreshed = () => {
  const h = new Headers();
  h.append('set-cookie', 'ekokod_at=NEWAT; Path=/; Max-Age=900; HttpOnly; Secure; SameSite=Strict');
  h.append('set-cookie', 'ekokod_rt=NEWRT; Path=/; HttpOnly; Secure; SameSite=Strict');
  return new Response(null, { status: 204, headers: h });
};
const failed = (code: string) => {
  const h = new Headers({ 'content-type': 'application/json' });
  if (code !== 'token_rotated') {
    h.append('set-cookie', 'ekokod_at=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Strict');
    h.append('set-cookie', 'ekokod_rt=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Strict');
  }
  return new Response(JSON.stringify({ error: { code, message: code } }), {
    status: 401,
    headers: h,
  });
};

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  vi.stubEnv('EKOKOD_INTERNAL_API_URL', INTERNAL);
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe('middleware', () => {
  it('tells the layout which path it guarded (R208)', async () => {
    const res = await middleware(request('/ekorm/consumption?x=1', 'ekokod_at=AT'));
    expect(res.headers.get('x-middleware-request-x-ekokod-path')).toBe('/ekorm/consumption?x=1');
  });

  it.each(['/', '/about', '/references', '/documents', '/toolkit', '/pricing', '/request-demo', '/contact', '/blog', '/blog/elektrik-faturam-neden-yuksek-1', '/bill-calculator'])(
    'serves the public page %s without a session (F12b)',
    async (path) => {
      const res = await middleware(request(path));
      expect(res.headers.get('location')).toBeNull();
      expect(res.headers.get('x-middleware-next')).toBe('1');
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it('does not treat a public prefix as a public page', async () => {
    const res = await middleware(request('/about-us'));
    expect(res.headers.get('location')).toContain('/auth/login');
  });

  it.each([
    ['/site', '/'],
    ['/site/billCalculate', '/bill-calculator'],
    ['/site/request-demo', '/request-demo'],
    ['/site/blog/detail/elektrik-faturam-neden-yuksek-2', '/blog/elektrik-faturam-neden-yuksek-2'],
    ['/site/unknown', '/'],
    // Review: crafted legacy URLs never leave the site.
    ['/site//evil.com', '/'],
    ['/site/blog/detail/%2F%2Fevil.com', '/blog/%2F%2Fevil.com'],
  ])('moves the legacy URL %s to %s permanently', async (from, to) => {
    const res = await middleware(request(from));
    expect(res.status).toBe(308);
    expect(res.headers.get('location')).toBe(`https://app.test${to}`);
  });

  it('lets auth pages through without cookies', async () => {
    const res = await middleware(request('/auth/login'));
    expect(res.headers.get('location')).toBeNull();
    expect(res.headers.get('x-middleware-next')).toBe('1');
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('sends a visitor without cookies to login with next', async () => {
    const res = await middleware(request('/ekorm/consumption?month=2026-08'));
    expect(res.status).toBe(307);
    expect(res.headers.get('location')).toBe(
      'https://app.test/auth/login?next=%2Fekorm%2Fconsumption%3Fmonth%3D2026-08',
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('passes a request with an access cookie straight through', async () => {
    const res = await middleware(request('/ekorm', 'ekokod_at=AT; ekokod_rt=RT'));
    expect(res.headers.get('x-middleware-next')).toBe('1');
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('refreshes server-side with the device headers and continues with the new cookies', async () => {
    fetchMock.mockResolvedValue(refreshed());
    const res = await middleware(request('/ekorm/consumption', 'NEXT_LOCALE=en; ekokod_rt=OLDRT'));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe(`${INTERNAL}/api/v1/auth/refresh`);
    expect(init.method).toBe('POST');
    const sent = new Headers(init.headers);
    expect(sent.get('cookie')).toBe('NEXT_LOCALE=en; ekokod_rt=OLDRT');
    expect(sent.get('user-agent')).toBe(UA);
    expect(sent.get('accept-language')).toBe('en');
    expect(sent.get('x-forwarded-for')).toBe('203.0.113.7');

    expect(res.headers.get('x-middleware-next')).toBe('1');
    const setCookies = res.headers.getSetCookie();
    expect(setCookies.some((c) => c.startsWith('ekokod_at=NEWAT'))).toBe(true);
    expect(setCookies.some((c) => c.startsWith('ekokod_rt=NEWRT'))).toBe(true);
    // The page render in this same request sees the fresh cookies.
    const forwarded = res.headers.get('x-middleware-request-cookie');
    expect(forwarded).toContain('ekokod_at=NEWAT');
    expect(forwarded).toContain('ekokod_rt=NEWRT');
    expect(forwarded).toContain('NEXT_LOCALE=en');
    expect(forwarded).not.toContain('OLDRT');
  });

  it('sends a device mismatch to login with the reason and expired cookies', async () => {
    fetchMock.mockResolvedValue(failed('device_mismatch'));
    const res = await middleware(request('/ekorm', 'ekokod_rt=RT'));
    expect(res.status).toBe(307);
    expect(res.headers.get('location')).toBe('https://app.test/auth/login?reason=device_mismatch');
    const setCookies = res.headers.getSetCookie();
    expect(setCookies.some((c) => /^ekokod_rt=;.*Max-Age=0/.test(c))).toBe(true);
    expect(setCookies.some((c) => /^ekokod_at=;.*Max-Age=0/.test(c))).toBe(true);
  });

  it('sends any other refresh failure to login with next', async () => {
    fetchMock.mockResolvedValue(failed('session_revoked'));
    const res = await middleware(request('/ekorm/settings', 'ekokod_rt=RT'));
    expect(res.headers.get('location')).toBe(
      'https://app.test/auth/login?next=%2Fekorm%2Fsettings',
    );
  });

  it('sends the user to login when the API is unreachable', async () => {
    fetchMock.mockRejectedValue(new TypeError('fetch failed'));
    const res = await middleware(request('/ekorm', 'ekokod_rt=RT'));
    expect(res.headers.get('location')).toBe('https://app.test/auth/login?next=%2Fekorm');
  });

  it('retries once when a parallel request already rotated the token', async () => {
    fetchMock.mockResolvedValue(failed('token_rotated'));
    const first = await middleware(request('/ekorm/settings', 'ekokod_rt=RT'));
    expect(first.headers.get('location')).toBe('https://app.test/ekorm/settings');
    expect(first.headers.getSetCookie().some((c) => c.startsWith('ekokod_retry=1'))).toBe(true);

    fetchMock.mockResolvedValue(failed('token_rotated'));
    const second = await middleware(request('/ekorm/settings', 'ekokod_rt=RT; ekokod_retry=1'));
    expect(second.headers.get('location')).toBe(
      'https://app.test/auth/login?next=%2Fekorm%2Fsettings',
    );
  });
});

describe('security headers on every response (F15a R452)', () => {
  const nonceOf = (csp: string | null) => /'nonce-([^']+)'/.exec(csp ?? '')?.[1];

  it('a public page gets the CSP, and its render gets the same nonce', async () => {
    const res = await middleware(request('/about'));
    const csp = res.headers.get('content-security-policy');
    expect(nonceOf(csp)).toBeTruthy();
    expect(res.headers.get('x-middleware-request-x-nonce')).toBe(nonceOf(csp));
    expect(res.headers.get('x-frame-options')).toBe('DENY');
  });

  it('a signed-in page keeps its guarded path and gets a nonce too', async () => {
    const res = await middleware(request('/ekorm/consumption', 'ekokod_at=AT'));
    expect(res.headers.get('x-middleware-request-x-ekokod-path')).toBe('/ekorm/consumption');
    expect(res.headers.get('x-middleware-request-x-nonce')).toBe(nonceOf(res.headers.get('content-security-policy')));
  });

  it('a redirect carries the headers as well, and every request draws a new nonce', async () => {
    const a = await middleware(request('/ekorm/consumption'));
    expect(a.status).toBe(307);
    const b = await middleware(request('/auth/login'));
    expect(nonceOf(a.headers.get('content-security-policy'))).toBeTruthy();
    expect(nonceOf(a.headers.get('content-security-policy'))).not.toBe(nonceOf(b.headers.get('content-security-policy')));
  });
});
