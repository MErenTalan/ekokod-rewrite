// @vitest-environment node
import { describe, expect, it } from 'vitest';

import { contentSecurityPolicy, newNonce, securityHeaders } from './csp';

describe('the content security policy (R452)', () => {
  it('draws a fresh 128-bit nonce each time', () => {
    const a = newNonce();
    expect(atob(a)).toHaveLength(16);
    expect(newNonce()).not.toBe(a);
  });

  it('runs only nonce-bearing scripts in production', () => {
    const csp = contentSecurityPolicy('abc', { NODE_ENV: 'production' });
    expect(csp).toContain("script-src 'self' 'nonce-abc' 'strict-dynamic'");
    expect(csp).not.toContain('unsafe-eval');
    expect(csp).not.toMatch(/script-src[^;]*unsafe-inline/);
    for (const d of ["object-src 'none'", "frame-ancestors 'none'", "base-uri 'self'", "form-action 'self'", "worker-src 'self' blob:"]) {
      expect(csp).toContain(d);
    }
  });

  it('adds the map tile origin, and the analytics origin only when that flag is on', () => {
    const env = { NEXT_PUBLIC_MAP_TILE_URL: 'https://tiles.example.com/{z}/{x}/{y}.png', EKOKOD_ANALYTICS_SRC: 'https://stats.example.com/s.js' };
    const off = contentSecurityPolicy('n', env);
    expect(off).toContain('img-src \'self\' data: blob: https://tiles.example.com');
    expect(off).toContain('connect-src \'self\' https://tiles.example.com');
    expect(off).not.toContain('stats.example.com');
    const on = contentSecurityPolicy('n', { ...env, EKOKOD_FEATURE_ANALYTICS: 'true' });
    expect(on).toContain("'strict-dynamic' https://stats.example.com");
  });

  it('lets React refresh evaluate code in development only', () => {
    expect(contentSecurityPolicy('n', { NODE_ENV: 'development' })).toContain("'unsafe-eval'");
  });

  it('sends HSTS only behind an HTTPS public URL (Q-L4)', () => {
    expect(securityHeaders('x', { EKOKOD_PUBLIC_URL: 'http://localhost:3000' })['Strict-Transport-Security']).toBeUndefined();
    const h = securityHeaders('x', { EKOKOD_PUBLIC_URL: 'https://app.example.com' });
    expect(h['Strict-Transport-Security']).toBe('max-age=31536000; includeSubDomains');
    expect(h['X-Frame-Options']).toBe('DENY');
    expect(h['X-Content-Type-Options']).toBe('nosniff');
  });
});
