import createNextIntlPlugin from 'next-intl/plugin';
import type { NextConfig } from 'next';

const withNextIntl = createNextIntlPlugin('./src/i18n/request.ts');

const nextConfig: NextConfig = {
  output: 'standalone',
  reactStrictMode: true,
  poweredByHeader: false,
  // The blog reads its Markdown at request time; standalone output must ship it (F12b).
  outputFileTracingIncludes: { '/blog/[slug]': ['./content/blog/**'] },
};

export default withNextIntl(nextConfig);
