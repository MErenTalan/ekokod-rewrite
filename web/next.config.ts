import createNextIntlPlugin from 'next-intl/plugin';
import type { NextConfig } from 'next';

const withNextIntl = createNextIntlPlugin('./src/i18n/request.ts');

const nextConfig: NextConfig = {
  output: 'standalone',
  reactStrictMode: true,
  poweredByHeader: false,
  // Same origin for the browser: /api/v1 is proxied to the Go API, so the auth cookies stay first-party (R168).
  async rewrites() {
    const api = process.env.EKOKOD_INTERNAL_API_URL ?? 'http://localhost:8080';
    return [{ source: '/api/v1/:path*', destination: `${api}/api/v1/:path*` }];
  },
};

export default withNextIntl(nextConfig);
