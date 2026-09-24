import type { SitePage } from '@/lib/site/paths';

export type SiteNavKey = 'about' | 'toolkit' | 'references' | 'documents' | 'blog' | 'calculator' | 'pricing' | 'contact';
export type SiteNavLink = { key: SiteNavKey; href: SitePage };

const LINKS: SiteNavLink[] = [
  { key: 'about', href: '/about' },
  { key: 'toolkit', href: '/toolkit' },
  { key: 'references', href: '/references' },
  { key: 'documents', href: '/documents' },
  { key: 'blog', href: '/blog' },
  { key: 'calculator', href: '/bill-calculator' },
  { key: 'pricing', href: '/pricing' },
  { key: 'contact', href: '/contact' },
];

/** The header and footer links; pricing only when its flag is on (01 §7.19). */
export function siteNav(pricing: boolean): SiteNavLink[] {
  return LINKS.filter((l) => pricing || l.key !== 'pricing');
}

export function isCurrent(pathname: string, href: string): boolean {
  return pathname === href || pathname.startsWith(`${href}/`);
}
