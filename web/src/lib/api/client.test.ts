import { describe, expect, it, vi } from 'vitest';

import { createApiClient } from './client';

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const err = (status: number, code: string) => json(status, { error: { code, message: code } });
const me = { user: { id: 'u' }, permissions: ['nav.core'] };

function setup(respond: (url: string, req: Request) => Response | Promise<Response>) {
  const calls: { url: string; method: string; headers: Headers }[] = [];
  const fetchImpl = vi.fn(async (req: Request) => {
    calls.push({ url: new URL(req.url).pathname, method: req.method, headers: req.headers });
    return respond(new URL(req.url).pathname, req);
  });
  const navigate = vi.fn();
  const { client, refreshOnce } = createApiClient({
    baseUrl: 'http://app.test',
    fetch: fetchImpl,
    navigate,
    currentPath: () => '/ekorm/consumption?x=1',
    locale: () => 'en',
  });
  return { client, refreshOnce, calls, navigate };
}

describe('api client', () => {
  it('refreshes once on token_expired and retries the request', async () => {
    let authed = false;
    const { client, calls, navigate } = setup((url) => {
      if (url === '/api/v1/auth/refresh') {
        authed = true;
        return new Response(null, { status: 204 });
      }
      return authed ? json(200, me) : err(401, 'token_expired');
    });
    const { data } = await client.GET('/api/v1/auth/me');
    expect(data).toEqual(me);
    expect(calls.map((c) => c.url)).toEqual([
      '/api/v1/auth/me',
      '/api/v1/auth/refresh',
      '/api/v1/auth/me',
    ]);
    expect(calls[0].headers.get('Accept-Language')).toBe('en');
    expect(navigate).not.toHaveBeenCalled();
  });

  it('refreshes once when the access cookie is gone (unauthorized)', async () => {
    let authed = false;
    const { client, calls } = setup((url) => {
      if (url === '/api/v1/auth/refresh') {
        authed = true;
        return new Response(null, { status: 204 });
      }
      return authed ? json(200, me) : err(401, 'unauthorized');
    });
    await client.GET('/api/v1/auth/me');
    expect(calls.filter((c) => c.url === '/api/v1/auth/refresh')).toHaveLength(1);
  });

  it('shares one refresh between concurrent 401s', async () => {
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    let authed = false;
    const { client, calls } = setup(async (url) => {
      if (url === '/api/v1/auth/refresh') {
        await gate;
        authed = true;
        return new Response(null, { status: 204 });
      }
      return authed ? json(200, me) : err(401, 'token_expired');
    });
    const both = Promise.all([client.GET('/api/v1/auth/me'), client.GET('/api/v1/auth/sessions')]);
    await vi.waitFor(() =>
      expect(calls.filter((c) => c.url === '/api/v1/auth/refresh')).toHaveLength(1),
    );
    release();
    const [a, b] = await both;
    expect(a.response.status).toBe(200);
    expect(b.response.status).toBe(200);
    expect(calls.filter((c) => c.url === '/api/v1/auth/refresh')).toHaveLength(1);
  });

  it('retries a mutation with its original body', async () => {
    let authed = false;
    const bodies: string[] = [];
    const { client } = setup(async (url, req) => {
      if (url === '/api/v1/auth/refresh') {
        authed = true;
        return new Response(null, { status: 204 });
      }
      bodies.push(await req.text());
      return authed ? new Response(null, { status: 204 }) : err(401, 'token_expired');
    });
    await client.PATCH('/api/v1/profile', { body: { locale: 'en' } });
    expect(bodies).toEqual(['{"locale":"en"}', '{"locale":"en"}']);
  });

  it('sends the user to login with next when the refresh fails', async () => {
    const { client, navigate } = setup((url) =>
      url === '/api/v1/auth/refresh' ? err(401, 'session_revoked') : err(401, 'token_expired'),
    );
    const { response } = await client.GET('/api/v1/auth/me');
    expect(response.status).toBe(401);
    expect(navigate).toHaveBeenCalledWith('/auth/login?next=%2Fekorm%2Fconsumption%3Fx%3D1');
  });

  it('sends device mismatch to login with the reason and never refreshes', async () => {
    const { client, calls, navigate } = setup(() => err(401, 'device_mismatch'));
    await client.GET('/api/v1/auth/me');
    expect(calls).toHaveLength(1);
    expect(navigate).toHaveBeenCalledWith('/auth/login?reason=device_mismatch');
  });

  it('leaves login failures to the form', async () => {
    const { client, calls, navigate } = setup(() => err(401, 'invalid_credentials'));
    const { error } = await client.POST('/api/v1/auth/login', {
      body: { email: 'a@b.c', password: 'x', remember_me: false },
    });
    expect(error?.error.code).toBe('invalid_credentials');
    expect(calls).toHaveLength(1);
    expect(navigate).not.toHaveBeenCalled();
  });

  it('never refreshes or redirects for a public auth endpoint, whatever its code', async () => {
    const { client, calls, navigate } = setup(() => err(401, 'token_expired'));
    await client.POST('/api/v1/auth/reset-password', { body: { token: 't', password: 'p' } });
    expect(calls).toHaveLength(1);
    expect(navigate).not.toHaveBeenCalled();
  });

  it('does not loop when the retried request is still unauthorised', async () => {
    const { client, calls, navigate } = setup((url) =>
      url === '/api/v1/auth/refresh'
        ? new Response(null, { status: 204 })
        : err(401, 'token_expired'),
    );
    await client.GET('/api/v1/auth/me');
    expect(calls).toHaveLength(3);
    expect(navigate).toHaveBeenCalledWith('/auth/login?next=%2Fekorm%2Fconsumption%3Fx%3D1');
  });
});
