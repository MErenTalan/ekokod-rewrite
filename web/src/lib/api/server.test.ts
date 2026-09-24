// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const incoming = new Headers();
const jar = new Map<string, string>();
vi.mock('next/headers', () => ({
  headers: async () => incoming,
  cookies: async () => ({
    get: (name: string) => (jar.has(name) ? { name, value: jar.get(name) } : undefined),
  }),
}));

const { getMe, getSession, serverApi } = await import('./server');

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  vi.stubEnv('EKOKOD_INTERNAL_API_URL', 'http://api.internal:18080');
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  for (const k of [...incoming.keys()]) incoming.delete(k);
  jar.clear();
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe('serverApi', () => {
  it('calls the internal URL with the session, device and locale headers', async () => {
    incoming.set('cookie', 'ekokod_at=AT; NEXT_LOCALE=en');
    incoming.set('user-agent', 'UA/1');
    incoming.set('accept-language', 'tr-TR');
    incoming.set('x-forwarded-for', '203.0.113.7');
    incoming.set('host', 'app.test');
    jar.set('NEXT_LOCALE', 'en');
    fetchMock.mockResolvedValue(Response.json({ id: 'u' }));
    await (await serverApi()).GET('/api/v1/auth/me');
    const request = fetchMock.mock.calls[0][0] as Request;
    expect(request.url).toBe('http://api.internal:18080/api/v1/auth/me');
    expect(request.headers.get('cookie')).toBe('ekokod_at=AT; NEXT_LOCALE=en');
    expect(request.headers.get('user-agent')).toBe('UA/1');
    expect(request.headers.get('x-forwarded-for')).toBe('203.0.113.7');
    expect(request.headers.get('accept-language')).toBe('en');
    expect(request.headers.get('host')).not.toBe('app.test');
  });
});

describe('getMe', () => {
  it('returns null when the API refuses or is down', async () => {
    fetchMock.mockResolvedValueOnce(
      Response.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 }),
    );
    expect(await getMe()).toBeNull();
  });

  it('keeps the refusal code so the layout can explain a device mismatch', async () => {
    fetchMock.mockResolvedValueOnce(Response.json({ error: { code: 'device_mismatch', message: 'x' } }, { status: 401 }));
    expect(await getSession()).toEqual({ me: null, code: 'device_mismatch' });
  });
});
