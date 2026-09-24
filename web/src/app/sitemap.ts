import type { MetadataRoute } from 'next';

import { POSTS } from '@/lib/site/blog';
import { siteFeatures } from '@/lib/site/features';
import { siteUrl, sitemapEntries } from '@/lib/site/seo';

export const dynamic = 'force-dynamic';

export default function sitemap(): MetadataRoute.Sitemap {
  return sitemapEntries(siteUrl(), siteFeatures().pricing, POSTS.map((p) => p.slug));
}
