import type { MetadataRoute } from 'next';

import { siteUrl } from '@/lib/site/seo';

export const dynamic = 'force-dynamic';

export default function robots(): MetadataRoute.Robots {
  const base = siteUrl();
  return {
    rules: [{ userAgent: '*', allow: '/', disallow: ['/ekorm', '/auth', '/api'] }],
    sitemap: `${base}/sitemap.xml`,
  };
}
