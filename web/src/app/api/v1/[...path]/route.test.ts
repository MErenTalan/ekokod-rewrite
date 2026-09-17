// @vitest-environment node
import { NextRequest } from 'next/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { GET, POST } from './route';

vi.mock('next/headers', () => ({ headers: async () => new Headers(), cookies: async () => ({ get: () => undefined }) }));

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  vi.stubEnv('EKOKOD_INTERNAL_API_URL', 'http://api.internal:18080');
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

const ctx = (...path: string[]) => ({ params: Promise.resolve({ path }) });

describe('/api/v1 proxy', () => {
  it('forwards method, path, query, body and session headers at runtime', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    const request = new NextRequest('https://app.test/api/v1/auth/login?x=1&y=%C5%9F', {
      method: 'POST',
      headers: {
        cookie: 'ekokod_rt=RT',
        'user-agent': 'UA/1',
        'content-type': 'application/json',
        'x-forwarded-for': '203.0.113.7',
        'idempotency-key': 'k1',
        connection: 'keep-alive',
      },
      body: '{"email":"a@b.c"}',
    });
    const res = await POST(request, ctx('auth', 'login'));
    expect(res.status).toBe(204);
    const [url, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(url.toString()).toBe('http://api.internal:18080/api/v1/auth/login?x=1&y=%C5%9F');
    expect(init.method).toBe('POST');
    const sent = new Headers(init.headers);
    expect(sent.get('cookie')).toBe('ekokod_rt=RT');
    expect(sent.get('user-agent')).toBe('UA/1');
    expect(sent.get('x-forwarded-for')).toBe('203.0.113.7');
    expect(sent.get('idempotency-key')).toBe('k1');
    expect(sent.get('connection')).toBeNull();
    expect(sent.get('host')).toBeNull();
    expect(await new Response(init.body).text()).toBe('{"email":"a@b.c"}');
  });

  it('keeps every Set-Cookie and the status, drops hop-by-hop and encoding headers', async () => {
    const upstream = new Headers({ 'content-type': 'application/json', 'content-encoding': 'gzip', 'retry-after': '60' });
    upstream.append('set-cookie', 'ekokod_at=A; Path=/; HttpOnly');
    upstream.append('set-cookie', 'ekokod_rt=R; Path=/; HttpOnly');
    fetchMock.mockResolvedValue(new Response('{"error":{"code":"rate_limited"}}', { status: 429, headers: upstream }));
    const res = await GET(new NextRequest('https://app.test/api/v1/auth/me'), ctx('auth', 'me'));
    expect(res.status).toBe(429);
    expect(res.headers.getSetCookie()).toEqual(['ekokod_at=A; Path=/; HttpOnly', 'ekokod_rt=R; Path=/; HttpOnly']);
    expect(res.headers.get('retry-after')).toBe('60');
    expect(res.headers.get('content-encoding')).toBeNull();
    expect(await res.text()).toBe('{"error":{"code":"rate_limited"}}');
  });

  it('refuses dot segments so nothing outside /api/v1 is reachable', async () => {
    const res = await GET(new NextRequest('https://app.test/api/v1/x'), ctx('..', 'health', 'ready'));
    expect(res.status).toBe(404);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('encodes each segment', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 404 }));
    await GET(new NextRequest('https://app.test/api/v1/x'), ctx('buildings', 'a/b'));
    expect((fetchMock.mock.calls[0][0] as URL).pathname).toBe('/api/v1/buildings/a%2Fb');
  });

  it('answers 502 with an envelope when the API is down', async () => {
    fetchMock.mockRejectedValue(new TypeError('fetch failed'));
    const res = await GET(new NextRequest('https://app.test/api/v1/auth/me'), ctx('auth', 'me'));
    expect(res.status).toBe(502);
    expect((await res.json()).error.code).toBe('api_unreachable');
  });
});
