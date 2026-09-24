/**
 * F15a R452: the web app's security headers and its nonce-based Content Security Policy.
 * Pure functions of the nonce and the environment, so the middleware and its tests agree.
 */
type Env = Record<string, string | undefined>;

/** A fresh 128-bit nonce, base64. */
export function newNonce(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  let s = '';
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s);
}

function originOf(url: string | undefined): string | null {
  if (!url) return null;
  try {
    const u = new URL(url.replace(/\{[^}]*\}/g, '0'));
    return u.protocol === 'https:' || u.protocol === 'http:' ? u.origin : null;
  } catch {
    return null;
  }
}

/** Q-L2's policy. The map tile origin and an opted-in analytics origin are the only extra sources. */
export function contentSecurityPolicy(nonce: string, env: Env = process.env): string {
  const tiles = originOf(env.NEXT_PUBLIC_MAP_TILE_URL);
  const analytics = env.EKOKOD_FEATURE_ANALYTICS === 'true' ? originOf(env.EKOKOD_ANALYTICS_SRC) : null;
  const dev = env.NODE_ENV === 'development';
  const extra = (...sources: (string | null)[]) => sources.filter((s): s is string => Boolean(s));
  const directives: [string, string[]][] = [
    ['default-src', ["'self'"]],
    ['script-src', ["'self'", `'nonce-${nonce}'`, "'strict-dynamic'", ...extra(analytics), ...(dev ? ["'unsafe-eval'"] : [])]],
    ['style-src', ["'self'", "'unsafe-inline'"]],
    ['img-src', ["'self'", 'data:', 'blob:', ...extra(tiles)]],
    ['font-src', ["'self'", 'data:']],
    ['connect-src', ["'self'", ...extra(tiles, analytics)]],
    ['worker-src', ["'self'", 'blob:']],
    ['object-src', ["'none'"]],
    ['base-uri', ["'self'"]],
    ['form-action', ["'self'"]],
    ['frame-ancestors', ["'none'"]],
  ];
  return directives.map(([name, values]) => `${name} ${values.join(' ')}`).join('; ');
}

/** Every response's security headers (HSTS only behind an HTTPS public URL, Q-L4). */
export function securityHeaders(csp: string, env: Env = process.env): Record<string, string> {
  const headers: Record<string, string> = {
    'Content-Security-Policy': csp,
    'X-Content-Type-Options': 'nosniff',
    'Referrer-Policy': 'strict-origin-when-cross-origin',
    'X-Frame-Options': 'DENY',
    'Permissions-Policy': 'camera=(), microphone=(), geolocation=(), payment=()',
  };
  if (env.EKOKOD_PUBLIC_URL?.startsWith('https://')) headers['Strict-Transport-Security'] = 'max-age=31536000; includeSubDomains';
  return headers;
}
