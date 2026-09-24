/** The public site's pages (01 §7.19); every other page needs a session. */
export const SITE_PAGES = [
  '/',
  '/about',
  '/references',
  '/documents',
  '/toolkit',
  '/pricing',
  '/request-demo',
  '/contact',
  '/blog',
  '/bill-calculator',
] as const;
export type SitePage = (typeof SITE_PAGES)[number];

export function isPublicPath(pathname: string): boolean {
  if ((SITE_PAGES as readonly string[]).includes(pathname)) return true;
  return /^\/blog\/[^/]+$/.test(pathname);
}

const LEGACY: Record<string, string> = {
  billCalculate: '/bill-calculator',
  blog2: '/blog',
};

/** Legacy `/site/*` URLs move permanently to their new paths; unknown ones land on the homepage. */
export function legacyRedirect(pathname: string): string | null {
  if (pathname !== '/site' && !pathname.startsWith('/site/')) return null;
  const rest = pathname.slice('/site/'.length);
  const detail = /^blog\/detail\/([^/]+)$/.exec(rest);
  if (detail) return `/blog/${detail[1]}`;
  if (LEGACY[rest]) return LEGACY[rest];
  const page = `/${rest}`;
  return page !== '/' && (SITE_PAGES as readonly string[]).includes(page) ? page : '/';
}
