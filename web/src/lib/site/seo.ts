import type { MetadataRoute } from 'next';

import { SITE_PAGES } from './paths';

type Env = Record<string, string | undefined>;

export function siteUrl(env: Env = process.env): string {
  return (env.EKOKOD_PUBLIC_URL ?? 'http://localhost:3000').replace(/\/+$/, '');
}

export function sitemapEntries(base: string, pricing: boolean, articles: string[]): MetadataRoute.Sitemap {
  const pages = SITE_PAGES.filter((p) => pricing || p !== '/pricing');
  return [
    ...pages.map((p) => ({ url: `${base}${p}`, changeFrequency: 'monthly' as const, priority: p === '/' ? 1 : 0.7 })),
    ...articles.map((slug) => ({ url: `${base}/blog/${slug}`, changeFrequency: 'yearly' as const, priority: 0.6 })),
  ];
}
