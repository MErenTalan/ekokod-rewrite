import type { NextRequest } from 'next/server';

import { internalApiUrl } from '@/lib/api/server';

// R168 same-origin API. A route handler, not next.config rewrites: rewrites are baked in at build time,
// and the image must follow EKOKOD_INTERNAL_API_URL at runtime.
export const dynamic = 'force-dynamic';

const HOP_BY_HOP = new Set([
  'connection', 'keep-alive', 'proxy-authenticate', 'proxy-authorization', 'te', 'trailer', 'transfer-encoding', 'upgrade',
  'host', 'content-length',
]);
// fetch has already decoded the body.
const RESPONSE_DROPPED = new Set([...HOP_BY_HOP, 'content-encoding', 'set-cookie']);

type Context = { params: Promise<{ path: string[] }> };

async function proxy(request: NextRequest, { params }: Context): Promise<Response> {
  const { path } = await params;
  if (path.some((segment) => segment === '.' || segment === '..' || segment === '')) {
    return Response.json({ error: { code: 'not_found', message: 'Not found.' } }, { status: 404 });
  }
  const target = new URL(`${internalApiUrl()}/api/v1/${path.map(encodeURIComponent).join('/')}`);
  target.search = request.nextUrl.search;

  const headers = new Headers();
  request.headers.forEach((value, name) => {
    if (!HOP_BY_HOP.has(name)) headers.set(name, value);
  });
  const init: RequestInit & { duplex?: 'half' } = { method: request.method, headers, redirect: 'manual', cache: 'no-store' };
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    init.body = request.body;
    init.duplex = 'half';
  }

  let upstream: Response;
  try {
    upstream = await fetch(target, init);
  } catch {
    return Response.json({ error: { code: 'api_unreachable', message: 'The API is unreachable.' } }, { status: 502 });
  }
  const out = new Headers();
  upstream.headers.forEach((value, name) => {
    if (!RESPONSE_DROPPED.has(name)) out.set(name, value);
  });
  for (const cookie of upstream.headers.getSetCookie()) out.append('set-cookie', cookie);
  return new Response(upstream.body, { status: upstream.status, statusText: upstream.statusText, headers: out });
}

export { proxy as DELETE, proxy as GET, proxy as HEAD, proxy as PATCH, proxy as POST, proxy as PUT };
