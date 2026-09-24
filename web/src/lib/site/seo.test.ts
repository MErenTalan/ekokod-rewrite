import { describe, expect, it } from 'vitest';

import { siteUrl, sitemapEntries } from './seo';

describe('siteUrl', () => {
  it('uses the public URL without a trailing slash, localhost otherwise', () => {
    expect(siteUrl({ EKOKOD_PUBLIC_URL: 'https://ekokod.com/' })).toBe('https://ekokod.com');
    expect(siteUrl({})).toBe('http://localhost:3000');
  });
});

describe('sitemapEntries', () => {
  it('lists every public page and article, pricing only behind its flag', () => {
    const urls = sitemapEntries('https://ekokod.com', false, ['a-1']).map((e) => e.url);
    expect(urls).toContain('https://ekokod.com/');
    expect(urls).toContain('https://ekokod.com/bill-calculator');
    expect(urls).toContain('https://ekokod.com/blog/a-1');
    expect(urls).not.toContain('https://ekokod.com/pricing');
    expect(sitemapEntries('https://ekokod.com', true, []).map((e) => e.url)).toContain('https://ekokod.com/pricing');
  });
});
